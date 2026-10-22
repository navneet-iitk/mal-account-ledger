# In-Memory Account Ledger Core

An enterprise-grade, append-only in-memory account ledger core implemented in Go, designed to model bi-temporal transaction posting, multi-currency precision (AED and BHD), authorization hold lifecycles, value-dated backdating, overdraft fee assessments, and interest accrual with exact Day 6 capitalization.

---

## 1. Quick Start

### Prerequisites
* Go 1.21+ (tested on Go 1.25)
* Standard library only (zero external third-party dependencies)

### Run the 6-Day Event Stream Replay
To replay the event stream (`E1` through `E10`) across Days 1 to 6 and print the daily closing state reports:
```bash
go run .
```
*(Or alternatively: `go run ./cmd/ledger`)*

### Run the Test Suite

#### 1. Run all passing unit tests:
```bash
go test -v ./pkg/ledger -skip TestDesignFlaw
```
All core tests will pass, validating daily balances, hold states, backdated projections, BHD instalment partitioning, exact interest sums, and append-only reversal integrity.

#### 2. Run the designated annotated failing test:
```bash
go test -v ./pkg/ledger -run TestDesignFlaw
```
This runs the intentional failing test (`TestDesignFlaw_NaiveReversalRestoresPreE7Balance`), exposing the flaw in the prompt's naive assertion that *"After E9, all balances and fees return to their pre-E7 values"*. The test output contains an inline annotation explaining why fee integrity is preserved in an append-only ledger.

---

## 2. Reading the Daily Output

Each day (Day 1 through Day 6) outputs a structured report containing four required sections plus interest tracking:

```
================================================================================
                              END OF DAY 5 REPORT                              
================================================================================

--- CLOSING LEDGER BALANCES (COMMITTED) ---
  * ACC-001 (AED):
      Closing Ledger Balance: -AED 360.00
      Active Holds (Pending): AED 0.00
      Available Balance:      -AED 360.00
  * ACC-002 (BHD):
      Closing Ledger Balance: BHD 10.000
      Active Holds (Pending): BHD 0.000
      Available Balance:      BHD 10.000

--- FEE ASSESSMENTS (DAY 5) ---
  * [FEE-OD-D5-ACC-001] Account: ACC-001 | Amount: AED 25.00 | Value Date: Day 5 | Overdraft fee for negative closing balance (-AED 335.00)

--- AUTHORIZATION STATES (AS OF DAY 5) ---
  * Auth-B | Account: ACC-001 | Hold: AED 90.00 | Status: REJECTED | Created: Day 5
  * Auth-A | Account: ACC-001 | Hold: AED 200.00 | Status: SETTLED | Created: Day 2 | Settled by: E5

--- ERRORS & REJECTIONS (DAY 5) ---
  * Event E8 (Account: ACC-001): insufficient available balance: required AED 90.00, available -AED 335.00

--- INTEREST ACCRUALS (DAY 5) ---
  * ACC-002: Base: BHD 10.000 | Rate: 0.04% | Accrued: BHD 0.004 (exact: 4.0000 fils)
================================================================================
```

### Key Field Explanations:
1. **Closing Ledger Balance**: Net accumulated settled entries in the `COMMITTED` phase with $\text{ValueDay} \le \text{CurrentDay}$.
2. **Active Holds (Pending)**: Total active reservations in the `PENDING_OUTGOING` phase.
3. **Available Balance**: $\text{Closing Ledger Balance} - \text{Active Holds}$.
4. **Fee Assessments**: Overdraft fees (AED 25.00) booked on negative closing balances, effective the day assessed.
5. **Authorization States**: Lifecycle tracking for all registered authorizations (`PENDING`, `SETTLED`, `REJECTED`).
6. **Errors & Rejections**: Transparent log of rejected transactions (e.g. `Auth-B` rejected due to insufficient available funds).
7. **Day 6 Capitalization**: A single credit entry posting the exact sum of daily rounded interest accruals ($\sum \text{RoundedFils}$).

---

## 3. Repository Structure & Deliverables

```
mal.ai/
├── .gitignore                 # Excluded build artifacts (*.pdf, *.html)
├── README.md                  # This file: execution guide and architecture summary
├── NUMBERS.md                 # Mathematical and economic justification for every constant
├── AMBIGUITIES.md             # 7 core banking ambiguities surfaced and resolved
├── REJECTED.md                # Prompt criteria explicitly refused with formal proofs
├── WORKLOG.md                 # Real, chronological engineering log
├── go.mod                     # Go module definition
├── main.go                    # Entrypoint CLI runner
├── pkg/
│   └── ledger/
│       ├── types.go           # Domain models, currencies, and posting phases
│       ├── engine.go          # Append-only engine, bi-temporal projection, fee rules
│       ├── replay.go          # Event stream (E1-E10) orchestrator
│       ├── printer.go         # Daily state report renderer
│       ├── ledger_test.go     # Comprehensive unit test suite (all passing)
│       └── design_flaw_test.go# Annotated failing tests exposing architectural trade-offs
└── architecture_and_trade_offs_doc/
    └── ARCHITECTURE_AND_TRADE_OFFS.md # 4 required architectural sections (compiled to PDF)
```

---

## 4. Architectural Summary

* **Posting Phase Model**: Rather than maintaining ad-hoc mutable hold variables, balances are modeled using posting dimensions (`COMMITTED`, `PENDING_OUTGOING`, `PENDING_INCOMING`).
* **Bi-Temporal Projection**: The ledger log is strictly append-only. Queries support bi-temporal evaluation: $\text{Balance}(\text{as\_of\_booking\_day}, \text{for\_value\_day})$. Backdated transactions (`E7`) never mutate past log entries.
* **Exact Integer Arithmetic**: All monetary quantities are represented as `int64` minor units (fils: 1 AED = 100 fils, 1 BHD = 1,000 fils). Floating-point arithmetic is strictly excluded to prevent currency drift.