package ledger

// GetStandardEventStream returns the canonical 10-event stream from the problem statement.
func GetStandardEventStream() []Event {
	return []Event{
		{
			ID:          "E1",
			Day:         1,
			Type:        EventCredit,
			AccountID:   "ACC-001",
			Amount:      120000, // AED 1,200.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    1,
			Notes:       "Credit AED 1,200.00",
		},
		{
			ID:          "E2",
			Day:         1,
			Type:        EventDebit,
			AccountID:   "ACC-001",
			Amount:      95000, // AED 950.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    1,
			Notes:       "Debit AED 950.00",
		},
		{
			ID:          "E3",
			Day:         2,
			Type:        EventAuthorization,
			AccountID:   "ACC-001",
			Amount:      20000, // AED 200.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    2,
			AuthID:      "Auth-A",
			Notes:       "Auth-A hold AED 200.00",
		},
		{
			ID:          "E4",
			Day:         3,
			Type:        EventCredit,
			AccountID:   "ACC-001",
			Amount:      40000, // AED 400.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    3,
			Notes:       "Credit AED 400.00",
		},
		{
			ID:          "E5",
			Day:         4,
			Type:        EventSettlement,
			AccountID:   "ACC-001",
			Amount:      18500, // AED 185.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    4,
			AuthID:      "Auth-A",
			Notes:       "Auth-A settles for AED 185.00",
		},
		{
			ID:          "E6",
			Day:         4,
			Type:        EventSettlement,
			AccountID:   "ACC-001",
			Amount:      18000, // AED 180.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    4,
			AuthID:      "Auth-Z",
			Notes:       "Auth-Z settles for AED 180.00 (no preceding auth)",
		},
		{
			ID:          "E7",
			Day:         5,
			Type:        EventDebit,
			AccountID:   "ACC-001",
			Amount:      62000, // AED 620.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    2, // BACK-DATED TO DAY 2
			Notes:       "Back-dated debit AED 620.00 (value_date: Day 2)",
		},
		{
			ID:          "E8",
			Day:         5,
			Type:        EventAuthorization,
			AccountID:   "ACC-001",
			Amount:      9000, // AED 90.00 in fils
			Currency:    CurrencyAED,
			ValueDay:    5,
			AuthID:      "Auth-B",
			Notes:       "Auth-B hold AED 90.00",
		},
		{
			ID:          "E10",
			Day:         5,
			Type:        EventCredit,
			AccountID:   "ACC-002",
			Amount:      10000, // BHD 10.000 in fils (3 decimals = 10,000 fils)
			Currency:    CurrencyBHD,
			ValueDay:    5,
			Instalments: 3,
			Notes:       "Credit BHD 10.000 posted as three equal instalments",
		},
		{
			ID:            "E9",
			Day:           6,
			Type:          EventReversal,
			AccountID:     "ACC-001",
			Currency:      CurrencyAED,
			ValueDay:      2,
			TargetEventID: "E7",
			Notes:         "Reversal of E7 (value_date: Day 2)",
		},
	}
}

// ReplayRunner coordinates the multi-day replay.
type ReplayRunner struct {
	Engine *Engine
	Events []Event
}

// NewReplayRunner initializes the runner with standard accounts and events.
func NewReplayRunner(cfg Config) *ReplayRunner {
	eng := NewEngine(cfg)

	// ACC-001 — AED, opening balance 0.00
	acc1 := NewAccount("ACC-001", CurrencyAED, TSideLiability, 1)
	eng.RegisterAccount(acc1)

	// ACC-002 — BHD, opening balance 0.000
	acc2 := NewAccount("ACC-002", CurrencyBHD, TSideLiability, 1)
	eng.RegisterAccount(acc2)

	return &ReplayRunner{
		Engine: eng,
		Events: GetStandardEventStream(),
	}
}

// ReplayDay executes all events scheduled for the specified day and runs EOD.
func (r *ReplayRunner) ReplayDay(day int) {
	for _, evt := range r.Events {
		if evt.Day == day {
			_ = r.Engine.ProcessEvent(evt)
		}
	}
	r.Engine.RunEndOfDay(day)
}

// ReplayAll executes the full 6-day simulation.
func (r *ReplayRunner) ReplayAll() {
	for d := 1; d <= 6; d++ {
		r.ReplayDay(d)
	}
}
