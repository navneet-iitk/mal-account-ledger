package ledger

import (
	"fmt"
	"math"
)

// Currency represents currency specification and minor-unit precision.
type Currency string

const (
	CurrencyAED Currency = "AED" // 2 decimal places (1 AED = 100 fils)
	CurrencyBHD Currency = "BHD" // 3 decimal places (1 BHD = 1000 fils)
)

// Precision returns the number of decimal places for the currency.
func (c Currency) Precision() int {
	switch c {
	case CurrencyBHD:
		return 3
	default:
		return 2
	}
}

// ScaleFactor returns the multiplication factor from major to minor units (fils).
func (c Currency) ScaleFactor() int64 {
	switch c {
	case CurrencyBHD:
		return 1000
	default:
		return 100
	}
}

// Format converts minor units (int64) into standard currency string representation.
func (c Currency) Format(amount int64) string {
	scale := c.ScaleFactor()
	precision := c.Precision()
	negative := amount < 0
	absAmount := amount
	if negative {
		absAmount = -absAmount
	}

	major := absAmount / scale
	minor := absAmount % scale

	var sign string
	if negative {
		sign = "-"
	}

	formatStr := fmt.Sprintf("%s%%s %%d.%%0%dd", sign, precision)
	return fmt.Sprintf(formatStr, c, major, minor)
}

// Phase represents the posting lifecycle phase dimension.
type Phase string

const (
	PhaseCommitted       Phase = "COMMITTED"
	PhasePendingOutgoing Phase = "PENDING_OUTGOING"
	PhasePendingIncoming Phase = "PENDING_INCOMING"
)

// TSide represents whether an account is an ASSET or LIABILITY on the bank's general ledger.
type TSide string

const (
	TSideAsset     TSide = "ASSET"
	TSideLiability TSide = "LIABILITY" // Customer deposits (CASA) are liabilities to the bank
)

// EventType defines the nature of the posting instruction.
type EventType string

const (
	EventCredit        EventType = "CREDIT"
	EventDebit         EventType = "DEBIT"
	EventAuthorization EventType = "AUTHORIZATION"
	EventSettlement    EventType = "SETTLEMENT"
	EventReversal      EventType = "REVERSAL"
)

// HoldStatus tracks the lifecycle of an authorization hold.
type HoldStatus string

const (
	HoldStatusPending  HoldStatus = "PENDING"
	HoldStatusSettled  HoldStatus = "SETTLED"
	HoldStatusReleased HoldStatus = "RELEASED"
	HoldStatusRejected HoldStatus = "REJECTED"
)

// Hold represents an authorization hold on customer funds.
type Hold struct {
	AuthID      string
	AccountID   string
	Amount      int64 // in minor units
	Currency    Currency
	CreatedDay  int
	ValueDay    int
	Status      HoldStatus
	SettledWith string // EventID of settlement
}

// LedgerEntry is an immutable, append-only record of a posted financial mutation.
type LedgerEntry struct {
	EntryID      string
	AccountID    string
	AccountAddress string // Defaults to "DEFAULT"
	Phase        Phase
	TSide        TSide
	Type         EventType
	Amount       int64 // Minor units. Positive increases balance, negative decreases.
	Currency     Currency
	BookingDay   int    // Day ingested by ledger (insertion day)
	ValueDay     int    // Effective value day
	ReferenceID  string // References AuthID, reversed EntryID, etc.
	Description  string
}

// BalanceRecord represents a point in the balance time-series.
type BalanceRecord struct {
	AccountID      string
	AccountAddress string
	Phase          Phase
	TSide          TSide
	Denomination   Currency
	Amount         int64 // Absolute cumulative amount in minor units
	EffectiveDay   int
	PIBID          string // Posting Instruction Batch ID / Event ID causing this record
}

// FeeAssessment records an assessed fee.
type FeeAssessment struct {
	FeeID        string
	AccountID    string
	Amount       int64 // in minor units (e.g. 2500 for AED 25.00)
	Currency     Currency
	AssessedDay  int
	ValueDay     int
	Reason       string
	Reversed     bool
}

// InterestAccrual records a single day's interest calculation.
type InterestAccrual struct {
	AccountID      string
	Day            int
	ClosingBalance int64   // Closing balance in minor units
	DailyRate      float64 // 0.0004 for 0.04%
	ExactFils      float64 // Unrounded exact accrual
	RoundedFils    int64   // Rounded daily accrual
}

// Event represents an inbound event from the event stream.
type Event struct {
	ID             string
	Day            int // Booking day
	Type           EventType
	AccountID      string
	Amount         int64 // minor units
	Currency       Currency
	ValueDay       int
	AuthID         string
	TargetEventID  string // for Reversal
	Instalments    int    // for E10 (e.g. 3)
	Notes          string
}

// Account represents a customer account.
type Account struct {
	ID           string
	Currency     Currency
	TSide        TSide
	OpeningDay   int

	// Balance time-series: list of balance mutations/snapshots
	BalanceHistory []BalanceRecord

	// Daily settled closing balances: ValueDay -> Closing Ledger Balance (COMMITTED)
	DailyLedgerBalances map[int]int64

	// Active holds: AuthID -> Hold
	Holds map[string]*Hold
}

// NewAccount creates an account with initial zero balances.
func NewAccount(id string, currency Currency, tside TSide, openingDay int) *Account {
	acc := &Account{
		ID:                  id,
		Currency:            currency,
		TSide:               tside,
		OpeningDay:          openingDay,
		BalanceHistory:      make([]BalanceRecord, 0),
		DailyLedgerBalances: make(map[int]int64),
		Holds:               make(map[string]*Hold),
	}

	// Initial balance snapshot at openingDay
	acc.BalanceHistory = append(acc.BalanceHistory, BalanceRecord{
		AccountID:      id,
		AccountAddress: "DEFAULT",
		Phase:          PhaseCommitted,
		TSide:          tside,
		Denomination:   currency,
		Amount:         0,
		EffectiveDay:   openingDay,
		PIBID:          "OPENING",
	})
	acc.DailyLedgerBalances[openingDay] = 0

	return acc
}

// RoundFils performs half-away-from-zero rounding on floating point fils.
func RoundFils(val float64) int64 {
	if val >= 0 {
		return int64(math.Floor(val + 0.5))
	}
	return int64(math.Ceil(val - 0.5))
}
