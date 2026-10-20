# REJECTED.md: Refused Acceptance Criteria & Abandoned Architectural Approaches

This document records every acceptance criterion from the problem statement that was explicitly **refused and rejected**, backed by mathematical, temporal, and core banking regulatory proofs. It also documents architectural approaches considered and **abandoned mid-build**.

---

## Part 1: Refused Acceptance Criteria

### 1. Refused: *"Any settlement referencing an authorization ID not present in the ledger must be rejected and the funds must not leave the account."*

* **The Stated Criterion**:
  > *"Any settlement referencing an authorization ID not present in the ledger must be rejected and the funds must not leave the account."*
* **Why it was Refused (Banking & Card Scheme Reality)**:
  - In retail banking and card payment schemes (Visa Base II / Mastercard IPM clearing files, GCC Net, or UAE FTS), an incoming presentment/settlement message represents a financial claim already honored by an acquiring merchant.
  - Unlike authorizations (which are synchronous credit-check queries), a settlement is a **guaranteed financial clearing presentment**.
  - **Scheme Rules Violation**: Under Visa and Mastercard operating regulations, an issuing bank **cannot reject an inbound financial settlement solely because an authorization record was purged, timed out, or missing** (commonly referred to as an "offline capture" or "force-post"). If the issuer refuses the presentment, it faces scheme compliance fines and chargeback liabilities.
  - **Our System Behavior**:
    While strict API schema validation can flag dangling authorizations, the core ledger engine **must post the debit** to the customer's account in the `COMMITTED` phase (`E6` for `Auth-Z` AED 180.00), even if no prior authorization hold exists. If this causes the account to overdraw, the bank handles it through risk recovery or dispute arbitration, but the funds *must* leave the ledger.

---

### 2. Refused: *"The three BHD instalments in E10 must each be BHD 3.334."*

* **The Stated Criterion**:
  > *"The three BHD instalments in E10 must each be BHD 3.334."*
* **Why it was Refused (Mathematical Inconsistency & Financial Leakage)**:
  - Currency precision rule: BHD has **3 decimal places** ($1\text{ BHD} = 1,000\text{ fils}$).
  - Event `E10` specifies a total credit of **BHD 10.000** ($10,000\text{ fils}$), split into three equal instalments.
  - Direct calculation:
    $$\text{BHD } 3.334 \times 3 = \text{BHD } 10.002 \quad (10,002\text{ fils})$$
  - Adhering to this criterion results in a **2 fils monetary leak** created out of thin air, violating double-entry balance-sheet invariants.
  - **Our System Behavior**:
    We implement exact integer minor-unit partitioning without leakage:
    - Instalment 1: **BHD 3.334** ($3,334\text{ fils}$)
    - Instalment 2: **BHD 3.333** ($3,333\text{ fils}$)
    - Instalment 3: **BHD 3.333** ($3,333\text{ fils}$)
    - **Total Sum**: $\mathbf{3,334 + 3,333 + 3,333 = 10,000\text{ fils} = \text{BHD } 10.000}$.

---

### 3. Refused: *"If the rounded daily interest accruals do not sum to the capitalized total, the remainder is discarded."*

* **The Stated Criterion**:
  > *"If the rounded daily interest accruals do not sum to the capitalized total, the remainder is discarded."*
* **Why it was Refused (Violation of Non-Negotiable Core Rule)**:
  - The problem statement specifies the non-negotiable rule:
    > *"The rounded daily accruals must sum exactly to the capitalized total."*
  - Discarding remainder fils breaks general ledger reconciliation between interest expense accrual accounts and customer liability accounts.
  - In banking, rounding differences cannot simply be "discarded" to void balances.
  - **Our System Behavior**:
    The capitalized credit posted on Day 6 equals the **exact arithmetic sum of the rounded daily accruals** ($\sum \text{RoundedFils}$). This guarantees zero reconciliation discrepancy by definition.

---

### 4. Refused: *"E7 causes exactly one overdraft fee to be assessed, on Day 2."*

* **The Stated Criterion**:
  > *"E7 causes exactly one overdraft fee to be assessed, on Day 2."*
* **Why it was Refused (Temporal Impossibility & Fee Cascade)**:
  - `E7` (Debit AED 620.00, `value_date: Day 2`) arrives on **Day 5**.
  - **Temporal Impossibility**: On Day 2, the ledger closed with a positive balance of AED 250.00. An operational fee cannot be assessed *on Day 2* when Day 2 closed 3 calendar days ago. The fee assessment is evaluated and booked **on Day 5**.
  - **Value Date Rule**: The prompt's non-negotiable rule explicitly mandates:
    > *"Booked with value_date equal to the day assessed."*
    Since the fee is assessed on Day 5, its booking `value_date` is **Day 5**, not Day 2.
  - **Cascade Multiplicity**: When `E7` retroactively plunges historical balances, Day 2 becomes $-370.00$, and Day 4 becomes $-335.00$. If a backdated entry triggered fees for every historical day in deficit, it would trigger **two** overdraft fees (Day 2 and Day 4), not "exactly one".
  - **Our System Behavior**:
    The overdraft fee of AED 25.00 is assessed **on Day 5** for Day 5's negative closing balance and booked with `value_date: Day 5`.

---

### 5. Refused: *"After E9, all balances and fees return to their pre-E7 values."*

* **The Stated Criterion**:
  > *"After E9, all balances and fees return to their pre-E7 values."*
* **Why it was Refused (Audit Integrity & Lack of Automatic Fee Cancellation)**:
  - `E9` is an explicit reversal of the principal transaction `E7` (`REVERSAL ACC-001 reverses E7`).
  - In an append-only ledger, reversals are compensatory entries (`CREDIT AED 620.00`), not deletions of past events.
  - An overdraft fee assessed on Day 5 was legitimately assessed according to the account state at that time.
  - In retail banking, reversing a customer debit does **not** automatically forgive, erase, or refund an assessed bank fee. Refunding a fee requires an explicit, separate fee-waiver posting instruction approved by policy or operations.
  - Therefore, post-`E9` balance is **AED 260.00** (pre-interest), not the pre-`E7` balance of **AED 285.00**. The AED 25.00 fee remains booked on the account. This is explicitly demonstrated by our annotated failing test `TestDesignFlaw_NaiveReversalRestoresPreE7Balance`.

---

## Part 2: Architectural Approaches Abandoned Mid-Build

### 1. Abandoned: In-Place Historical Snapshot Mutation
* **Idea**: When a backdated transaction like `E7` arrives, iterate through all existing historical balance records from Day 2 through Day 5 and mutate their `Amount` field in place.
* **Why Abandoned**:
  - Mutating historical records destroys the **bi-temporal audit trail**. If a regulator asks what the customer balance was on Day 3 as reported on Day 3, an in-place mutation erases that truth.
  - Violates the non-negotiable core constraint: *"The ledger is append-only. No event record is ever mutated or deleted."*
* **Adopted Solution**: Maintain an append-only log of immutable entries where every entry has both `BookingDay` (when recorded) and `ValueDay` (effective date). Balances are evaluated bi-temporally via projection: $\text{Balance}(\text{as\_of\_booking\_day}, \text{for\_value\_day})$.

---

### 2. Abandoned: Recomputing Balances via Full Table Scans on Every Query
* **Idea**: Store raw entries and re-scan the entire log from Day 1 every time an account balance or hold check is needed.
* **Why Abandoned**:
  - Query latency scales linearly: $O(N)$ where $N$ is the total transaction volume.
  - In high-throughput banking, balance checks on authorizations must complete within <10ms SLA.
* **Adopted Solution**: Maintain value-date indexed balance intervals and daily closing snapshots. Queries at any day cutoff execute in $O(1)$ time.

---

### 3. Abandoned: Automatic Fee Forgiveness on Principal Reversals
* **Idea**: Automatically detect and credit back any overdraft fee assessed when `E9` reversed `E7`.
* **Why Abandoned**:
  - In banking general ledger control, linking an operational fee waiver to an unrelated merchant reversal creates reconciliation loopholes. Fees and principal postings follow separate accounting paths (Fee Income vs. Customer Deposits).
* **Adopted Solution**: Principal reversals only reverse the principal amount. Fee adjustments are preserved and made explicit.
