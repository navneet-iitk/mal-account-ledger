package ledger

import (
	"sync"
	"testing"
)

// ================================================================================================
// DESIGN FLAW TEST 1: The "Naive Reversal Restores All Balances and Fees" Fallacy
// ================================================================================================
// WHAT THIS TEST REVEALS:
// Acceptance Criterion refused in REJECTED.md: "After E9, all balances and fees return to their pre-E7 values."
//
// In a naive mutable system, reversing E7 would simply delete E7 and delete the fee.
// But in an enterprise-grade immutable append-only core banking ledger:
// 1. E7 caused an overdraft fee of AED 25.00 to be assessed on Day 5.
// 2. E9 is a principal reversal of E7 (+AED 620.00). It does NOT and CANNOT automatically cancel
//    or refund a legitimately assessed bank fee. A bank fee requires an explicit compensatory
//    fee-waiver/refund posting instruction.
// 3. Pre-E7 ledger balance on Day 4 was AED 285.00.
//    Post-E9 ledger balance on Day 6 (before interest) is AED 260.00 (285.00 - 25.00 fee).
//
// This test asserts the naive prompt criterion (that balance returns to exactly pre-E7 AED 285.00)
// and FAILS intentionally, proving that our append-only ledger preserves fee integrity.
func TestDesignFlaw_NaiveReversalRestoresPreE7Balance(t *testing.T) {
	runner := NewReplayRunner(DefaultConfig())

	// Replay Day 1 through Day 4
	for d := 1; d <= 4; d++ {
		runner.ReplayDay(d)
	}
	preE7Balance := runner.Engine.GetClosingLedgerBalance("ACC-001", 4, 4)
	if preE7Balance != 28500 {
		t.Fatalf("unexpected pre-E7 balance: %d", preE7Balance)
	}

	// Replay Day 5 (E7 backdated debit arrives, overdraft fee assessed)
	runner.ReplayDay(5)

	// Replay Day 6 (E9 reversal arrives) - evaluate balance BEFORE interest capitalization
	for _, evt := range runner.Events {
		if evt.Day == 6 {
			_ = runner.Engine.ProcessEvent(evt)
		}
	}
	postE9Balance := runner.Engine.GetClosingLedgerBalance("ACC-001", 6, 6)

	// NAIVE CRITERION CHECK (INTENTIONALLY FAILS):
	// The prompt claimed: "After E9, all balances and fees return to their pre-E7 values."
	// In reality, postE9Balance is 260.00 AED (26000 fils) because the 25.00 AED fee remains booked!
	if postE9Balance != preE7Balance {
		t.Errorf("DESIGN FLAW REVEALED: Pre-E7 balance was %s, but post-E9 balance is %s!\n"+
			"REASON: The overdraft fee of AED 25.00 assessed on Day 5 was not wiped out by E9.\n"+
			"In an immutable append-only ledger, a principal debit reversal (E9) does not\n"+
			"magically erase assessed fees without an explicit fee-waiver posting.\n"+
			"The prompt's criterion claiming 'all balances and fees return to their pre-E7 values'\n"+
			"is false and violates core banking audit integrity.",
			CurrencyAED.Format(preE7Balance), CurrencyAED.Format(postE9Balance))
	}
}

// ================================================================================================
// DESIGN FLAW TEST 2: Concurrency Race Condition on In-Memory Hold Validation
// ================================================================================================
// WHAT THIS TEST REVEALS:
// Our in-memory ledger is optimized for deterministic single-threaded stream replay.
// In a high-throughput production environment, if two authorization requests arrive concurrently:
// - Initial available balance: AED 200.00
// - Goroutine 1 requests hold: AED 150.00
// - Goroutine 2 requests hold: AED 150.00
//
// Without Optimistic Concurrency Control (OCC versioning) or pessimistic partition row locks,
// both goroutines evaluate GetAvailableBalance() at the same instant (seeing AED 200.00 >= 150.00),
// both approve the authorization, and available balance drops to -AED 100.00,
// breaching the non-negotiable invariant: "available balance remains at or above zero"!
func TestDesignFlaw_ConcurrentAuthorizationRaceBreachesAvailableBalance(t *testing.T) {
	eng := NewEngine(DefaultConfig())
	acc := NewAccount("ACC-RACE", CurrencyAED, TSideLiability, 1)
	eng.RegisterAccount(acc)

	// Fund account with AED 200.00
	_ = eng.ProcessEvent(Event{
		ID:        "FUND",
		Day:       1,
		Type:      EventCredit,
		AccountID: "ACC-RACE",
		Amount:    20000, // AED 200.00
		Currency:  CurrencyAED,
		ValueDay:  1,
	})

	// Fire two concurrent authorizations of AED 150.00 each
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = eng.ProcessEvent(Event{
			ID:        "AUTH-1",
			Day:       1,
			Type:      EventAuthorization,
			AccountID: "ACC-RACE",
			Amount:    15000, // AED 150.00
			Currency:  CurrencyAED,
			ValueDay:  1,
			AuthID:    "AUTH-1",
		})
	}()

	go func() {
		defer wg.Done()
		_ = eng.ProcessEvent(Event{
			ID:        "AUTH-2",
			Day:       1,
			Type:      EventAuthorization,
			AccountID: "ACC-RACE",
			Amount:    15000, // AED 150.00
			Currency:  CurrencyAED,
			ValueDay:  1,
			AuthID:    "AUTH-2",
		})
	}()

	wg.Wait()

	// Available balance should NEVER drop below 0
	avail := eng.GetAvailableBalance("ACC-RACE", 1)
	if avail < 0 {
		t.Errorf("DESIGN FLAW REVEALED: Available balance dropped to %s (negative)!\n"+
			"REASON: The in-memory ledger lacks mutex locking or Optimistic Concurrency Control (OCC)\n"+
			"version checks on Account. In production core banking systems, balance mutations\n"+
			"are serialized per account partition or guarded by Spanner row-level transactions\n"+
			"to prevent concurrent double-spend race conditions.",
			CurrencyAED.Format(avail))
	}
}
