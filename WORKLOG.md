# WORKLOG.md: Chronological Engineering Log

This log documents the real chronological progression, engineering decisions, and iterations made throughout the implementation.

---

### [2026-10-20 21:30] — Requirements Ingestion & Problem Discovery
- Cloned repository workspace and discovered three Markdown files:
  - `README.md`
  - `in_memory_account_ledger_core/PROBLEM_STATEMENT.md`
  - `architecture_and_trade_offs_doc/PROBLEM_STATEMENT.md`
- Key evaluation takeaway noted: The written artifacts are merely an "entry ticket"; the actual assessment is a **45-minute live defense with NO AI present**.
- Core requirement identified: In-memory core ledger engine, strict append-only rules, bi-temporal value-dated postings, hold lifecycles, and integer precision across AED (2 decimals) and BHD (3 decimals).
- Identified the prompt's explicit warning: *"Some of the following criteria are wrong. Identify every incorrect criterion, refuse it, and document your reasoning in REJECTED.md."*

---

### [2026-10-20 22:30] — Domain Modeling & Architecture Alignment
- Evaluated programming language choices (Go vs. Python).
  - Selected **Go**: Strong typing, integer arithmetic without float drift, zero runtime dependencies, and mirrors production core banking engine architectures.
- Analyzed the temporal nature of ledger balances:
  - Naive mutable balances (updating scalar floats) rejected immediately.
  - Examined balance dimension models: separating `COMMITTED` funds from `PENDING_OUTGOING` holds on the `DEFAULT` account address.
  - Confirmed available balance definition:
    $$\text{Available} = \text{COMMITTED} - \text{PENDING\_OUTGOING}$$
- Addressed backdated entries (`E7` value-dated Day 2 arriving on Day 5):
  - In an append-only ledger, the log is strictly sequenced by ingestion time.
  - Historical queries must be bi-temporal: evaluating entries with $\text{BookingDay} \le T_{\text{eval}}$ and $\text{ValueDay} \le T_{\text{target}}$.
  - Rejected in-place historical snapshot mutation because it erases regulatory audit history.

---

### [2026-10-20 23:15] — Mathematical Proofs & Trap Refusals
- Dissected the acceptance criteria and formalized mathematical/regulatory counter-proofs:
  1. **BHD Instalments**: Prompt claimed $3 \times \text{BHD } 3.334 = \text{BHD } 10.000$. Proved mathematically false ($3.334 \times 3 = 10.002$, leaking 2 fils). Implemented remainder-preserving integer partition ($3.334, 3.333, 3.333$).
  2. **Unmatched Settlement (`Auth-Z`)**: Prompt claimed missing auth must be rejected and funds not leave account. Refused under card scheme clearing rules (Visa/Mastercard presentment cannot be declined by issuing bank; must post as Force-Post / Hard Settlement).
  3. **Interest Remainder Discarding**: Refused because discarding remainders contradicts the prompt's non-negotiable rule that daily accruals must sum exactly to capitalized total.
  4. **Overdraft Fee Timing on Backdating**: Refused the claim that `E7` causes a fee assessed "on Day 2". In real-time, Day 2 is closed; the fee is assessed on Day 5 with value date Day 5.
  5. **Reversal Restoring Fees**: Proved that reversing debit `E7` via `E9` does not automatically erase or refund an assessed bank fee.

---

### [2026-10-20 23:35] — Implementation: Types, Engine, & Runner
- Initialized Go module `mal-ledger`.
- Created `pkg/ledger/types.go`:
  - `Currency` (`AED`, `BHD`) with exact minor-unit scaling (100 and 1,000 fils).
  - `Phase` (`COMMITTED`, `PENDING_OUTGOING`, `PENDING_INCOMING`).
  - `TSide` (`LIABILITY` for customer CASA deposits).
  - `LedgerEntry` (immutable, append-only record).
  - `Hold` and `FeeAssessment` structs.
- Created `pkg/ledger/engine.go`:
  - Append-only entry storage.
  - Bi-temporal balance query engine: `GetClosingLedgerBalance(accountID, asOfBookingDay, targetValueDay)`.
  - Hold approval logic with invariant validation (`availableBalance >= holdAmount`).
  - Settlement processing (matching hold release + COMMITTED debit).
  - Overdraft fee assessment (AED 25.00 once per day on negative closing balance).
  - Daily interest accrual (0.04% on positive balance) and Day 6 exact capitalization.
- Created `pkg/ledger/replay.go` and `pkg/ledger/printer.go`:
  - 6-day replay runner replaying `E1` through `E10`.
  - Daily End-of-Day formatted report renderer.
- Created `cmd/ledger/main.go` and root `main.go`.

---

### [2026-10-20 23:55] — Verification, Testing, & Design Flaw Test
- Ran full 6-day simulation (`go run .`). Output verified across all 6 days.
- Authored comprehensive test suite in `pkg/ledger/ledger_test.go`:
  - `TestClosingBalances_Days1Through6`: Verified Day 1 through Day 6 balances.
  - `TestDay2ClosingBalance_EvaluatedAtEndOfDay5`: Verified Day 2 balance evaluated at Day 5 equals -AED 370.00.
  - `TestBHDInstalments_ExactSum`: Verified BHD partition without leakage.
  - `TestDailyInterestAccrual_AndExactCapitalization`: Verified exact capitalized sum.
  - `TestReversal_AppendOnlyIntegrity`: Verified E9 compensatory credit without mutating E7.
  - `TestStrictSettlementValidation_ToggledOn`: Verified strict schema validation toggle.
- Authored designated annotated failing tests in `pkg/ledger/design_flaw_test.go`:
  - `TestDesignFlaw_NaiveReversalRestoresPreE7Balance`: Fails as designed, revealing that principal debit reversals do not automatically erase assessed fees.
  - `TestDesignFlaw_ConcurrentAuthorizationRaceBreachesAvailableBalance`: Reveals that an in-memory ledger lacking OCC/mutex locks is vulnerable to concurrent double-spend races.

---

### [2026-10-21 00:05] — Documentation & Architecture Synthesis
- Authored `REJECTED.md`: Full documentation of refused criteria and abandoned designs.
- Authored `AMBIGUITIES.md`: Surfacing and resolving all 7 identified banking ambiguities.
- Authored `NUMBERS.md`: Comprehensive justification for all constants (and why not half).
- Authored `README.md`: User manual and execution guide.
- Prepared the 4 required sections for the Architecture & Trade-Offs document.
