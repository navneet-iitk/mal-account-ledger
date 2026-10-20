package ledger

import (
	"fmt"
	"io"
	"strings"
)

// ReportPrinter renders the daily ledger state per day.
type ReportPrinter struct {
	Out io.Writer
}

// NewReportPrinter creates a printer targeting the specified writer.
func NewReportPrinter(out io.Writer) *ReportPrinter {
	return &ReportPrinter{Out: out}
}

// PrintDaySummary outputs the required daily report: closing balance, fees, auth states, and errors.
func (p *ReportPrinter) PrintDaySummary(eng *Engine, day int) {
	fmt.Fprintf(p.Out, "\n================================================================================\n")
	fmt.Fprintf(p.Out, "                              END OF DAY %d REPORT                              \n", day)
	fmt.Fprintf(p.Out, "================================================================================\n")

	// 1. Closing Ledger Balances
	fmt.Fprintf(p.Out, "\n--- CLOSING LEDGER BALANCES (COMMITTED) ---\n")
	for _, acc := range eng.sortedAccounts() {
		closingBal := eng.GetClosingLedgerBalance(acc.ID, day, day)
		availBal := eng.GetAvailableBalance(acc.ID, day)
		holds := eng.GetActiveHolds(acc.ID, day)

		fmt.Fprintf(p.Out, "  * %s (%s):\n", acc.ID, acc.Currency)
		fmt.Fprintf(p.Out, "      Closing Ledger Balance: %s\n", acc.Currency.Format(closingBal))
		fmt.Fprintf(p.Out, "      Active Holds (Pending): %s\n", acc.Currency.Format(holds))
		fmt.Fprintf(p.Out, "      Available Balance:      %s\n", acc.Currency.Format(availBal))
	}

	// 2. Fee Assessments
	fmt.Fprintf(p.Out, "\n--- FEE ASSESSMENTS (DAY %d) ---\n", day)
	dayFees := make([]FeeAssessment, 0)
	for _, fee := range eng.FeeAssessments {
		if fee.AssessedDay == day {
			dayFees = append(dayFees, fee)
		}
	}
	if len(dayFees) == 0 {
		fmt.Fprintf(p.Out, "  (No fees assessed on Day %d)\n", day)
	} else {
		for _, fee := range dayFees {
			fmt.Fprintf(p.Out, "  * [%s] Account: %s | Amount: %s | Value Date: Day %d | %s\n",
				fee.FeeID, fee.AccountID, fee.Currency.Format(fee.Amount), fee.ValueDay, fee.Reason)
		}
	}

	// 3. Authorization States
	fmt.Fprintf(p.Out, "\n--- AUTHORIZATION STATES (AS OF DAY %d) ---\n", day)
	hasAuths := false
	for _, acc := range eng.sortedAccounts() {
		for authID, hold := range acc.Holds {
			if hold.CreatedDay <= day {
				hasAuths = true
				fmt.Fprintf(p.Out, "  * %s | Account: %s | Hold: %s | Status: %s | Created: Day %d",
					authID, acc.ID, acc.Currency.Format(hold.Amount), hold.Status, hold.CreatedDay)
				if hold.Status == HoldStatusSettled {
					fmt.Fprintf(p.Out, " | Settled by: %s", hold.SettledWith)
				}
				fmt.Fprintln(p.Out)
			}
		}
	}
	if !hasAuths {
		fmt.Fprintf(p.Out, "  (No authorizations registered up to Day %d)\n", day)
	}

	// 4. Daily Errors & Rejections
	fmt.Fprintf(p.Out, "\n--- ERRORS & REJECTIONS (DAY %d) ---\n", day)
	dayErrors := make([]EventError, 0)
	for _, err := range eng.Errors {
		if err.Day == day {
			dayErrors = append(dayErrors, err)
		}
	}
	if len(dayErrors) == 0 {
		fmt.Fprintf(p.Out, "  (None)\n")
	} else {
		for _, err := range dayErrors {
			fmt.Fprintf(p.Out, "  * Event %s (Account: %s): %s\n", err.EventID, err.AccountID, err.Reason)
		}
	}

	// 5. Interest Accrual Tracking
	fmt.Fprintf(p.Out, "\n--- INTEREST ACCRUALS (DAY %d) ---\n", day)
	dayAccruals := make([]InterestAccrual, 0)
	for _, accr := range eng.InterestAccrual {
		if accr.Day == day {
			dayAccruals = append(dayAccruals, accr)
		}
	}
	if len(dayAccruals) == 0 {
		fmt.Fprintf(p.Out, "  (No interest accrued on Day %d - balance <= 0)\n", day)
	} else {
		for _, accr := range dayAccruals {
			acc, _ := eng.GetAccount(accr.AccountID)
			fmt.Fprintf(p.Out, "  * %s: Base: %s | Rate: 0.04%% | Accrued: %s (exact: %.4f fils)\n",
				accr.AccountID, acc.Currency.Format(accr.ClosingBalance), acc.Currency.Format(accr.RoundedFils), accr.ExactFils)
		}
	}

	// On Day 6: Show Capitalization Total
	if day == 6 {
		fmt.Fprintf(p.Out, "\n--- DAY 6 CAPITALIZATION EVENT ---\n")
		for _, entry := range eng.Entries {
			if strings.HasPrefix(entry.EntryID, "INT-CAP") {
				fmt.Fprintf(p.Out, "  * %s posted: %s credited to %s (%s)\n",
					entry.EntryID, entry.Currency.Format(entry.Amount), entry.AccountID, entry.Description)
			}
		}
	}

	fmt.Fprintf(p.Out, "================================================================================\n")
}
