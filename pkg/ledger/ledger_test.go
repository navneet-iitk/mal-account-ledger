package ledger

import (
	"testing"
)

func TestClosingBalances_Days1Through6(t *testing.T) {
	cfg := DefaultConfig()
	runner := NewReplayRunner(cfg)

	// Day 1
	runner.ReplayDay(1)
	balD1 := runner.Engine.GetClosingLedgerBalance("ACC-001", 1, 1)
	if balD1 != 25000 { // AED 250.00
		t.Fatalf("expected Day 1 closing balance AED 250.00 (25000 fils), got %d", balD1)
	}

	// Day 2
	runner.ReplayDay(2)
	balD2 := runner.Engine.GetClosingLedgerBalance("ACC-001", 2, 2)
	if balD2 != 25000 { // AED 250.00
		t.Fatalf("expected Day 2 closing balance AED 250.00, got %d", balD2)
	}
	availD2 := runner.Engine.GetAvailableBalance("ACC-001", 2)
	if availD2 != 5000 { // AED 50.00 (250 - 200 hold)
		t.Fatalf("expected Day 2 available balance AED 50.00, got %d", availD2)
	}

	// Day 3
	runner.ReplayDay(3)
	balD3 := runner.Engine.GetClosingLedgerBalance("ACC-001", 3, 3)
	if balD3 != 65000 { // AED 650.00
		t.Fatalf("expected Day 3 closing balance AED 650.00, got %d", balD3)
	}

	// Day 4
	runner.ReplayDay(4)
	balD4 := runner.Engine.GetClosingLedgerBalance("ACC-001", 4, 4)
	// E5 settled 185.00, E6 force-post settled 180.00 -> 650 - 185 - 180 = 285.00
	if balD4 != 28500 {
		t.Fatalf("expected Day 4 closing balance AED 285.00, got %d", balD4)
	}

	// Day 5
	runner.ReplayDay(5)
	// On Day 5, E7 (-620.00) backdated to Day 2 plunged Day 5 balance to -335.00 before fee.
	// Overdraft fee AED 25.00 assessed, closing at -360.00.
	balD5 := runner.Engine.GetClosingLedgerBalance("ACC-001", 5, 5)
	if balD5 != -36000 {
		t.Fatalf("expected Day 5 closing balance -AED 360.00, got %d", balD5)
	}

	// Auth-B must have been rejected on Day 5 due to negative balance
	acc1, _ := runner.Engine.GetAccount("ACC-001")
	authB, exists := acc1.Holds["Auth-B"]
	if !exists || authB.Status != HoldStatusRejected {
		t.Fatalf("expected Auth-B to be REJECTED, got %v", authB)
	}

	// Day 6
	runner.ReplayDay(6)
	// E9 reversed E7 (+620.00). Ledger before capitalization: -360.00 + 620.00 = 260.00.
	// Day 6 capitalized interest: 0.67 AED. Final closing: 260.67 AED.
	balD6 := runner.Engine.GetClosingLedgerBalance("ACC-001", 6, 6)
	if balD6 != 26067 {
		t.Fatalf("expected Day 6 final balance AED 260.67 (26067 fils), got %d", balD6)
	}

	// ACC-002 check: 10.000 BHD + 0.008 interest capitalization = 10.008 BHD
	balBHD := runner.Engine.GetClosingLedgerBalance("ACC-002", 6, 6)
	if balBHD != 10008 {
		t.Fatalf("expected ACC-002 final balance BHD 10.008 (10008 fils), got %d", balBHD)
	}
}

func TestDay2ClosingBalance_EvaluatedAtEndOfDay5(t *testing.T) {
	// Acceptance criterion:
	// "The Day 2 closing ledger balance, evaluated at end of Day 5 and before any fee is assessed, is AED −370.00."
	eng := NewEngine(DefaultConfig())
	acc := NewAccount("ACC-001", CurrencyAED, TSideLiability, 1)
	eng.RegisterAccount(acc)

	events := GetStandardEventStream()
	// Process events up to Day 5 (excluding fee booking)
	for _, evt := range events {
		if evt.Day <= 5 && evt.AccountID == "ACC-001" {
			_ = eng.ProcessEvent(evt)
		}
	}

	// Query Day 2 balance as known at end of Day 5 (before Day 5 EOD fee is assessed)
	// Entries with BookingDay <= 5 and ValueDay <= 2:
	// E1 (+1200.00), E2 (-950.00), E7 (-620.00) = -370.00 AED
	balDay2AsOf5 := eng.GetClosingLedgerBalance("ACC-001", 5, 2)
	expected := int64(-37000) // -370.00 in fils
	if balDay2AsOf5 != expected {
		t.Fatalf("expected Day 2 balance evaluated at end of Day 5 to be -37000 fils (-AED 370.00), got %d", balDay2AsOf5)
	}
}

func TestBHDInstalments_ExactSum(t *testing.T) {
	// Acceptance criterion test:
	// "The three BHD instalments in E10 must each be BHD 3.334." -> This was REFUSED.
	// 3.334 * 3 = 10.002 (leaks 2 fils).
	// The true partition must sum exactly to 10.000 fils without remainder leakage.
	total := int64(10000) // BHD 10.000
	parts := 3
	instalments := partitionMinorUnits(total, parts)

	if len(instalments) != 3 {
		t.Fatalf("expected 3 instalments, got %d", len(instalments))
	}

	var sum int64
	for _, inst := range instalments {
		sum += inst
	}
	if sum != total {
		t.Fatalf("instalments do not sum to total: got %d, expected %d", sum, total)
	}

	// First instalment carries remainder: 3334, 3333, 3333
	if instalments[0] != 3334 || instalments[1] != 3333 || instalments[2] != 3333 {
		t.Fatalf("unexpected instalment allocation: %v", instalments)
	}
}

func TestDailyInterestAccrual_AndExactCapitalization(t *testing.T) {
	// Rule: "The rounded daily accruals must sum exactly to the capitalized total."
	cfg := DefaultConfig()
	runner := NewReplayRunner(cfg)
	runner.ReplayAll()

	var sumRoundedACC1 int64
	for _, accr := range runner.Engine.InterestAccrual {
		if accr.AccountID == "ACC-001" {
			sumRoundedACC1 += accr.RoundedFils
		}
	}

	var capEntryACC1 *LedgerEntry
	for i := range runner.Engine.Entries {
		if runner.Engine.Entries[i].EntryID == "INT-CAP-D6-ACC-001" {
			capEntryACC1 = &runner.Engine.Entries[i]
			break
		}
	}

	if capEntryACC1 == nil {
		t.Fatal("expected Day 6 interest capitalization entry for ACC-001")
	}

	if capEntryACC1.Amount != sumRoundedACC1 {
		t.Fatalf("capitalized amount %d does not match sum of rounded daily accruals %d",
			capEntryACC1.Amount, sumRoundedACC1)
	}
}

func TestReversal_AppendOnlyIntegrity(t *testing.T) {
	// Verify that E9 reverses E7 via compensatory posting without mutating or deleting E7
	cfg := DefaultConfig()
	runner := NewReplayRunner(cfg)
	runner.ReplayAll()

	var e7Found, e9Found bool
	for _, entry := range runner.Engine.Entries {
		if entry.EntryID == "E7" {
			e7Found = true
			if entry.Amount != -62000 {
				t.Fatalf("E7 mutated: expected amount -62000, got %d", entry.Amount)
			}
		}
		if entry.EntryID == "E9" {
			e9Found = true
			if entry.Amount != 62000 {
				t.Fatalf("E9 incorrect compensatory amount: expected 62000, got %d", entry.Amount)
			}
			if entry.ReferenceID != "E7" {
				t.Fatalf("E9 should reference E7, got %s", entry.ReferenceID)
			}
		}
	}

	if !e7Found || !e9Found {
		t.Fatalf("expected both E7 and E9 to exist in append-only log (e7=%v, e9=%v)", e7Found, e9Found)
	}
}

func TestStrictSettlementValidation_ToggledOn(t *testing.T) {
	// When StrictAuthSettlementMatch is enabled, E6 (Auth-Z) is rejected because Auth-Z has no preceding authorization
	cfg := Config{StrictAuthSettlementMatch: true}
	eng := NewEngine(cfg)
	acc := NewAccount("ACC-001", CurrencyAED, TSideLiability, 1)
	eng.RegisterAccount(acc)

	evtE6 := Event{
		ID:        "E6",
		Day:       4,
		Type:      EventSettlement,
		AccountID: "ACC-001",
		Amount:    18000,
		Currency:  CurrencyAED,
		ValueDay:  4,
		AuthID:    "Auth-Z",
	}

	err := eng.ProcessEvent(evtE6)
	if err == nil {
		t.Fatal("expected E6 to be rejected under strict schema validation")
	}
}
