package ledger

import (
	"errors"
	"fmt"
	"sort"
)

// EventError represents an error or rejection encountered while processing an event.
type EventError struct {
	EventID   string
	Day       int
	AccountID string
	Reason    string
}

// Config controls engine behavioral flags.
type Config struct {
	// StrictAuthSettlementMatch: If true, rejects settlements without pre-existing auth (strict schema validation).
	// If false, accepts them as Scheme Force-Posts / Hard Settlements (Visa/Mastercard clearing rules).
	StrictAuthSettlementMatch bool

	// ReverseFeesOnReversal: If true, automatically generates a fee reversal posting when a backdated debit is reversed.
	ReverseFeesOnReversal bool
}

// DefaultConfig returns the recommended production banking configuration.
func DefaultConfig() Config {
	return Config{
		StrictAuthSettlementMatch: false, // Core banking must honor inbound scheme settlements (Auth-Z)
		ReverseFeesOnReversal:     false, // Banking principle: fees require explicit fee waiver/reversal postings
	}
}

// Engine encapsulates the append-only ledger state, accounts, and rule evaluations.
type Engine struct {
	Config          Config
	Accounts        map[string]*Account
	Entries         []LedgerEntry
	FeeAssessments  []FeeAssessment
	InterestAccrual []InterestAccrual
	Errors          []EventError
	CurrentDay      int

	// Track fees assessed per account per evaluated day to prevent duplicate fee assessments
	feesAssessedMap map[string]bool // key: "accountID:day"
}

// NewEngine initializes an empty ledger engine.
func NewEngine(cfg Config) *Engine {
	return &Engine{
		Config:          cfg,
		Accounts:        make(map[string]*Account),
		Entries:         make([]LedgerEntry, 0),
		FeeAssessments:  make([]FeeAssessment, 0),
		InterestAccrual: make([]InterestAccrual, 0),
		Errors:          make([]EventError, 0),
		CurrentDay:      1,
		feesAssessedMap: make(map[string]bool),
	}
}

// RegisterAccount adds an account to the ledger.
func (e *Engine) RegisterAccount(acc *Account) {
	e.Accounts[acc.ID] = acc
}

// GetAccount retrieves an account by ID.
func (e *Engine) GetAccount(id string) (*Account, error) {
	acc, ok := e.Accounts[id]
	if !ok {
		return nil, fmt.Errorf("account %s not found", id)
	}
	return acc, nil
}

// AppendEntry appends an immutable entry to the ledger log and updates balance time-series.
func (e *Engine) AppendEntry(entry LedgerEntry) {
	e.Entries = append(e.Entries, entry)

	acc, exists := e.Accounts[entry.AccountID]
	if !exists {
		return
	}

	// Calculate new cumulative balance for this phase and address
	currentComm := e.GetClosingLedgerBalance(entry.AccountID, entry.BookingDay, entry.ValueDay)

	acc.BalanceHistory = append(acc.BalanceHistory, BalanceRecord{
		AccountID:      entry.AccountID,
		AccountAddress: entry.AccountAddress,
		Phase:          entry.Phase,
		TSide:          entry.TSide,
		Denomination:   entry.Currency,
		Amount:         currentComm,
		EffectiveDay:   entry.ValueDay,
		PIBID:          entry.EntryID,
	})
}

// GetClosingLedgerBalance returns closing ledger balance (COMMITTED) evaluated at asOfBookingDay for entries with ValueDay <= targetValueDay.
// This supports bi-temporal queries: what was the balance for targetValueDay as known on asOfBookingDay?
func (e *Engine) GetClosingLedgerBalance(accountID string, asOfBookingDay int, targetValueDay int) int64 {
	var total int64
	for _, entry := range e.Entries {
		if entry.AccountID != accountID {
			continue
		}
		if entry.Phase != PhaseCommitted {
			continue
		}
		// Consider entries ingested on or before asOfBookingDay with ValueDay <= targetValueDay
		if entry.BookingDay <= asOfBookingDay && entry.ValueDay <= targetValueDay {
			total += entry.Amount
		}
	}
	return total
}

// GetActiveHolds returns the total amount of active holds (PENDING_OUTGOING) for an account as of a booking day.
func (e *Engine) GetActiveHolds(accountID string, asOfBookingDay int) int64 {
	acc, exists := e.Accounts[accountID]
	if !exists {
		return 0
	}

	var total int64
	for _, hold := range acc.Holds {
		if hold.CreatedDay <= asOfBookingDay && hold.Status == HoldStatusPending {
			total += hold.Amount
		}
	}
	return total
}

// GetAvailableBalance returns available balance: COMMITTED (as of current day) minus active holds.
func (e *Engine) GetAvailableBalance(accountID string, asOfBookingDay int) int64 {
	ledgerBal := e.GetClosingLedgerBalance(accountID, asOfBookingDay, asOfBookingDay)
	holds := e.GetActiveHolds(accountID, asOfBookingDay)
	return ledgerBal - holds
}

// ProcessEvent processes an incoming event in strict replay order.
func (e *Engine) ProcessEvent(evt Event) error {
	acc, err := e.GetAccount(evt.AccountID)
	if err != nil {
		e.logError(evt.ID, evt.Day, evt.AccountID, err.Error())
		return err
	}

	switch evt.Type {
	case EventCredit:
		return e.processCredit(acc, evt)

	case EventDebit:
		return e.processDebit(acc, evt)

	case EventAuthorization:
		return e.processAuthorization(acc, evt)

	case EventSettlement:
		return e.processSettlement(acc, evt)

	case EventReversal:
		return e.processReversal(acc, evt)

	default:
		err := fmt.Errorf("unknown event type: %s", evt.Type)
		e.logError(evt.ID, evt.Day, evt.AccountID, err.Error())
		return err
	}
}

func (e *Engine) processCredit(acc *Account, evt Event) error {
	// If instalments specified (e.g. E10), partition amount evenly in minor units
	if evt.Instalments > 1 {
		instalmentAmounts := partitionMinorUnits(evt.Amount, evt.Instalments)
		for idx, amt := range instalmentAmounts {
			subID := fmt.Sprintf("%s-%d", evt.ID, idx+1)
			entry := LedgerEntry{
				EntryID:        subID,
				AccountID:      acc.ID,
				AccountAddress: "DEFAULT",
				Phase:          PhaseCommitted,
				TSide:          acc.TSide,
				Type:           EventCredit,
				Amount:         amt,
				Currency:       acc.Currency,
				BookingDay:     evt.Day,
				ValueDay:       evt.ValueDay,
				ReferenceID:    evt.ID,
				Description:    fmt.Sprintf("Instalment %d/%d of %s", idx+1, evt.Instalments, evt.ID),
			}
			e.AppendEntry(entry)
		}
		return nil
	}

	entry := LedgerEntry{
		EntryID:        evt.ID,
		AccountID:      acc.ID,
		AccountAddress: "DEFAULT",
		Phase:          PhaseCommitted,
		TSide:          acc.TSide,
		Type:           EventCredit,
		Amount:         evt.Amount,
		Currency:       acc.Currency,
		BookingDay:     evt.Day,
		ValueDay:       evt.ValueDay,
		Description:    evt.Notes,
	}
	e.AppendEntry(entry)
	return nil
}

func (e *Engine) processDebit(acc *Account, evt Event) error {
	entry := LedgerEntry{
		EntryID:        evt.ID,
		AccountID:      acc.ID,
		AccountAddress: "DEFAULT",
		Phase:          PhaseCommitted,
		TSide:          acc.TSide,
		Type:           EventDebit,
		Amount:         -evt.Amount, // negative mutation on customer balance
		Currency:       acc.Currency,
		BookingDay:     evt.Day,
		ValueDay:       evt.ValueDay,
		Description:    evt.Notes,
	}
	e.AppendEntry(entry)
	return nil
}

func (e *Engine) processAuthorization(acc *Account, evt Event) error {
	// Rule: Approved only if available balance (ledger balance - active holds) remains >= 0 after hold applied
	availBal := e.GetAvailableBalance(acc.ID, evt.Day)
	if availBal < evt.Amount {
		reason := fmt.Sprintf("insufficient available balance: required %s, available %s",
			acc.Currency.Format(evt.Amount), acc.Currency.Format(availBal))
		e.logError(evt.ID, evt.Day, acc.ID, reason)

		acc.Holds[evt.AuthID] = &Hold{
			AuthID:     evt.AuthID,
			AccountID:  acc.ID,
			Amount:     evt.Amount,
			Currency:   acc.Currency,
			CreatedDay: evt.Day,
			ValueDay:   evt.ValueDay,
			Status:     HoldStatusRejected,
		}
		return fmt.Errorf("authorization %s rejected: %s", evt.AuthID, reason)
	}

	// Approved hold: recorded in PENDING_OUTGOING phase
	acc.Holds[evt.AuthID] = &Hold{
		AuthID:     evt.AuthID,
		AccountID:  acc.ID,
		Amount:     evt.Amount,
		Currency:   acc.Currency,
		CreatedDay: evt.Day,
		ValueDay:   evt.ValueDay,
		Status:     HoldStatusPending,
	}

	// Record balance point reflecting PENDING_OUTGOING
	acc.BalanceHistory = append(acc.BalanceHistory, BalanceRecord{
		AccountID:      acc.ID,
		AccountAddress: "DEFAULT",
		Phase:          PhasePendingOutgoing,
		TSide:          acc.TSide,
		Denomination:   acc.Currency,
		Amount:         evt.Amount,
		EffectiveDay:   evt.ValueDay,
		PIBID:          evt.ID,
	})

	return nil
}

func (e *Engine) processSettlement(acc *Account, evt Event) error {
	hold, hasHold := acc.Holds[evt.AuthID]

	if !hasHold {
		// Core banking schema validation vs Card Scheme clearing edge case:
		if e.Config.StrictAuthSettlementMatch {
			reason := fmt.Sprintf("settlement referencing unknown auth ID %s rejected (strict schema validation)", evt.AuthID)
			e.logError(evt.ID, evt.Day, acc.ID, reason)
			return errors.New(reason)
		}

		// In production card banking (Visa/Mastercard presentment), an issuing bank MUST accept force-posts/unmatched settlements
		entry := LedgerEntry{
			EntryID:        evt.ID,
			AccountID:      acc.ID,
			AccountAddress: "DEFAULT",
			Phase:          PhaseCommitted,
			TSide:          acc.TSide,
			Type:           EventSettlement,
			Amount:         -evt.Amount,
			Currency:       acc.Currency,
			BookingDay:     evt.Day,
			ValueDay:       evt.ValueDay,
			ReferenceID:    evt.AuthID,
			Description:    fmt.Sprintf("Force-post / Unmatched settlement for %s", evt.AuthID),
		}
		e.AppendEntry(entry)
		return nil
	}

	// Matching hold found: release hold (PENDING_OUTGOING -> 0) and debit COMMITTED
	hold.Status = HoldStatusSettled
	hold.SettledWith = evt.ID

	entry := LedgerEntry{
		EntryID:        evt.ID,
		AccountID:      acc.ID,
		AccountAddress: "DEFAULT",
		Phase:          PhaseCommitted,
		TSide:          acc.TSide,
		Type:           EventSettlement,
		Amount:         -evt.Amount,
		Currency:       acc.Currency,
		BookingDay:     evt.Day,
		ValueDay:       evt.ValueDay,
		ReferenceID:    evt.AuthID,
		Description:    fmt.Sprintf("Settlement matching %s (original hold %s, settled for %s)",
			evt.AuthID, acc.Currency.Format(hold.Amount), acc.Currency.Format(evt.Amount)),
	}
	e.AppendEntry(entry)
	return nil
}

func (e *Engine) processReversal(acc *Account, evt Event) error {
	// Find the targeted entry to reverse
	var targetEntry *LedgerEntry
	for i := range e.Entries {
		if e.Entries[i].EntryID == evt.TargetEventID && e.Entries[i].AccountID == acc.ID {
			targetEntry = &e.Entries[i]
			break
		}
	}

	if targetEntry == nil {
		reason := fmt.Sprintf("cannot reverse entry %s: not found in ledger", evt.TargetEventID)
		e.logError(evt.ID, evt.Day, acc.ID, reason)
		return errors.New(reason)
	}

	// Reversal appends a compensatory entry with inverted amount. The original entry is NEVER deleted.
	compensatoryAmount := -targetEntry.Amount
	entry := LedgerEntry{
		EntryID:        evt.ID,
		AccountID:      acc.ID,
		AccountAddress: targetEntry.AccountAddress,
		Phase:          PhaseCommitted,
		TSide:          acc.TSide,
		Type:           EventReversal,
		Amount:         compensatoryAmount,
		Currency:       acc.Currency,
		BookingDay:     evt.Day,
		ValueDay:       evt.ValueDay,
		ReferenceID:    evt.TargetEventID,
		Description:    fmt.Sprintf("Reversal of entry %s (%s)", evt.TargetEventID, acc.Currency.Format(targetEntry.Amount)),
	}
	e.AppendEntry(entry)

	// If configured to reverse fees on reversal, post fee reversal
	if e.Config.ReverseFeesOnReversal {
		for i := range e.FeeAssessments {
			fee := &e.FeeAssessments[i]
			if fee.AccountID == acc.ID && !fee.Reversed {
				fee.Reversed = true
				feeReversalEntry := LedgerEntry{
					EntryID:        fmt.Sprintf("REV-%s", fee.FeeID),
					AccountID:      acc.ID,
					AccountAddress: "DEFAULT",
					Phase:          PhaseCommitted,
					TSide:          acc.TSide,
					Type:           EventCredit,
					Amount:         fee.Amount,
					Currency:       fee.Currency,
					BookingDay:     evt.Day,
					ValueDay:       evt.ValueDay,
					ReferenceID:    fee.FeeID,
					Description:    fmt.Sprintf("Fee waiver/reversal for %s", fee.FeeID),
				}
				e.AppendEntry(feeReversalEntry)
			}
		}
	}

	return nil
}

// RunEndOfDay executes EOD lifecycle rules for the given day:
// 1. Evaluate closing ledger balance
// 2. Assess overdraft fee if negative
// 3. Calculate daily interest accrual on positive balance
// 4. On Day 6, capitalize accumulated interest as single credit
func (e *Engine) RunEndOfDay(day int) {
	e.CurrentDay = day

	for _, acc := range e.sortedAccounts() {
		closingBal := e.GetClosingLedgerBalance(acc.ID, day, day)
		acc.DailyLedgerBalances[day] = closingBal

		// Rule: Overdraft fee AED 25.00 assessed once per day per account when closing ledger balance is negative.
		// Booked with value_date equal to the day assessed.
		feeKey := fmt.Sprintf("%s:%d", acc.ID, day)
		if closingBal < 0 && !e.feesAssessedMap[feeKey] && acc.Currency == CurrencyAED {
			feeAmount := int64(2500) // AED 25.00 in fils
			feeID := fmt.Sprintf("FEE-OD-D%d-%s", day, acc.ID)

			fee := FeeAssessment{
				FeeID:       feeID,
				AccountID:   acc.ID,
				Amount:      feeAmount,
				Currency:    CurrencyAED,
				AssessedDay: day,
				ValueDay:    day,
				Reason:      fmt.Sprintf("Overdraft fee for negative closing balance (%s)", acc.Currency.Format(closingBal)),
			}
			e.FeeAssessments = append(e.FeeAssessments, fee)
			e.feesAssessedMap[feeKey] = true

			// Book fee debit to ledger with value_date equal to day assessed
			feeEntry := LedgerEntry{
				EntryID:        feeID,
				AccountID:      acc.ID,
				AccountAddress: "DEFAULT",
				Phase:          PhaseCommitted,
				TSide:          acc.TSide,
				Type:           EventDebit,
				Amount:         -feeAmount,
				Currency:       CurrencyAED,
				BookingDay:     day,
				ValueDay:       day,
				Description:    fee.Reason,
			}
			e.AppendEntry(feeEntry)

			// Update closing balance after fee assessment
			closingBal = e.GetClosingLedgerBalance(acc.ID, day, day)
			acc.DailyLedgerBalances[day] = closingBal
		}

		// Rule: Daily interest 0.04% per day on closing ledger balance, positive balances only.
		if closingBal > 0 {
			rate := 0.0004 // 0.04% per day
			exact := float64(closingBal) * rate
			rounded := RoundFils(exact)

			accrual := InterestAccrual{
				AccountID:      acc.ID,
				Day:            day,
				ClosingBalance: closingBal,
				DailyRate:      rate,
				ExactFils:      exact,
				RoundedFils:    rounded,
			}
			e.InterestAccrual = append(e.InterestAccrual, accrual)
		}

		// On Day 6: Capitalize accruals as a single credit at end of Day 6
		if day == 6 {
			var totalRoundedAccruals int64
			for _, accr := range e.InterestAccrual {
				if accr.AccountID == acc.ID {
					totalRoundedAccruals += accr.RoundedFils
				}
			}

			if totalRoundedAccruals > 0 {
				capEntry := LedgerEntry{
					EntryID:        fmt.Sprintf("INT-CAP-D6-%s", acc.ID),
					AccountID:      acc.ID,
					AccountAddress: "DEFAULT",
					Phase:          PhaseCommitted,
					TSide:          acc.TSide,
					Type:           EventCredit,
					Amount:         totalRoundedAccruals,
					Currency:       acc.Currency,
					BookingDay:     6,
					ValueDay:       6,
					Description:    fmt.Sprintf("Capitalized 6-day interest total of %s", acc.Currency.Format(totalRoundedAccruals)),
				}
				e.AppendEntry(capEntry)
				acc.DailyLedgerBalances[6] = e.GetClosingLedgerBalance(acc.ID, 6, 6)
			}
		}
	}
}

func (e *Engine) logError(eventID string, day int, accountID string, reason string) {
	e.Errors = append(e.Errors, EventError{
		EventID:   eventID,
		Day:       day,
		AccountID: accountID,
		Reason:    reason,
	})
}

func (e *Engine) sortedAccounts() []*Account {
	keys := make([]string, 0, len(e.Accounts))
	for k := range e.Accounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	res := make([]*Account, 0, len(keys))
	for _, k := range keys {
		res = append(res, e.Accounts[k])
	}
	return res
}

// partitionMinorUnits divides a total minor-unit amount into n equal parts without losing remainders.
// For example, 10,000 fils into 3 parts yields: 3,334, 3,333, 3,333 (sum = 10,000).
func partitionMinorUnits(total int64, parts int) []int64 {
	if parts <= 0 {
		return []int64{total}
	}
	base := total / int64(parts)
	remainder := total % int64(parts)

	res := make([]int64, parts)
	for i := 0; i < parts; i++ {
		res[i] = base
		if int64(i) < remainder {
			res[i]++
		}
	}
	return res
}
