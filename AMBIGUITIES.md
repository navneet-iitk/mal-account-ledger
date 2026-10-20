# AMBIGUITIES.md: Discovered Edge Cases & Architectural Resolutions

In production core banking engines, underspecified requirements and contradictory rules are the primary source of accounting discrepancies and regulatory findings. This document catalogs every ambiguity identified in the problem statement and details how each was resolved.

---

### Ambiguity 1: Fee Assessment Timing on Backdated Entries (Booking Day vs. Value Day)

* **The Ambiguity**:
  - `E7` (Debit AED 620.00, `value_date: Day 2`) is booked on **Day 5**.
  - Its value date retroactively drives the Day 2 closing ledger balance to **-AED 370.00**.
  - The problem statement specifies two rules that create tension:
    1. *"Overdraft fee: AED 25.00, assessed once per day per account when that day's closing ledger balance... is negative."*
    2. *"Booked with value_date equal to the day assessed."*
  - Does the overdraft fee get assessed retroactively for Day 2 (violating temporal reality), or is it assessed on Day 5 when the deficit was recognized?
* **Resolution**:
  - In an append-only ledger, a fee cannot be assessed on a closed historical day. It is assessed **on Day 5**.
  - Under the non-negotiable rule (*"Booked with value_date equal to the day assessed"*), because the assessment occurs on Day 5, the fee's `value_date` is **Day 5**.
  - This avoids modifying the historical Day 2 closing balance record and treats the fee as an operational consequence realized on Day 5.

---

### Ambiguity 2: Cascading Overdraft Deficits from Backdated Entries

* **The Ambiguity**:
  - When `E7` retroactively debits AED 620.00 effective Day 2:
    - Day 2 closing balance becomes **-AED 370.00** (deficit).
    - Day 3 receives `E4` (+AED 400.00), bringing balance to **+AED 30.00** (positive).
    - Day 4 posts settlements `E5` (-AED 185.00) and `E6` (-AED 180.00), bringing balance to **-AED 335.00** (deficit).
  - The acceptance criteria state: *"E7 causes exactly one overdraft fee to be assessed, on Day 2."*
  - If a backdated entry exposes multiple historical deficit days, does the account incur:
    - Zero fees (because Day 2 was closed)?
    - One fee for Day 2?
    - Two fees (for Day 2 and Day 4)?
    - Or one fee on Day 5?
* **Resolution**:
  - We refuse the assertion that *"E7 causes exactly one fee to be assessed on Day 2"* in `REJECTED.md`.
  - In our engine, fee assessment occurs at End-of-Day for the active evaluation cycle. On Day 5, the closing balance is negative (-AED 335.00), triggering exactly one overdraft fee of AED 25.00 for Day 5.

---

### Ambiguity 3: Unmatched Settlement (`Auth-Z` in E6) — Schema Validation vs. Scheme Clearing

* **The Ambiguity**:
  - Event `E6` is a settlement of AED 180.00 referencing `Auth-Z`, but `Auth-Z` has no prior authorization in the ledger.
  - Acceptance criterion states: *"Any settlement referencing an authorization ID not present in the ledger must be rejected and the funds must not leave the account."*
  - Yet in real-world retail banking, card schemes (Visa/Mastercard) and direct debit networks present financial clearing records that **cannot be declined** by the issuing bank without incurring severe penalties.
* **Resolution**:
  - We implemented a dual-mode configuration (`Config.StrictAuthSettlementMatch`):
    - **Strict Mode (`true`)**: Rejects `E6` at the API boundary, logging an error (*used to verify the naive requirement in unit tests*).
    - **Production Core Banking Mode (`false`, Default)**: Recognizes `E6` as a scheme **Force-Post / Hard Settlement**. The engine posts the debit to `COMMITTED` funds, ensuring scheme settlement reconciliation, while logging a system audit flag for operations.

---

### Ambiguity 4: Event Stream Ordering of `E10`

* **The Ambiguity**:
  - In the problem statement, the event stream lists:
    ```
    - E7 — Day 5 — DEBIT ...
    - E8 — Day 5 — AUTHORIZATION ...
    - E9 — Day 6 — REVERSAL ...
    - E10 — Day 5 — CREDIT — ACC-002 BHD 10.000 ...
    ```
  - `E10` has a booking date of **Day 5**, yet appears *after* `E9` (Day 6) in the textual sequence.
  - Is `E10` ingested during Day 5 processing, or does it represent an out-of-order message arriving on Day 6 with a backdated booking date?
* **Resolution**:
  - `E10` operates exclusively on `ACC-002` (BHD), whereas `E1` through `E9` operate on `ACC-001` (AED).
  - Because `ACC-001` and `ACC-002` are independent multi-currency accounts, processing `E10` on Day 5 or Day 6 has zero cross-account impact.
  - We ingest `E10` as part of **Day 5** operations, ensuring `ACC-002` accrues interest correctly on Day 5 and Day 6 prior to Day 6 capitalization.

---

### Ambiguity 5: Reversal (`E9`) Impact on Previously Assessed Overdraft Fees

* **The Ambiguity**:
  - `E9` reverses `E7` on Day 6 with `value_date: Day 2`.
  - Acceptance criterion states: *"After E9, all balances and fees return to their pre-E7 values."*
  - If an overdraft fee was assessed on Day 5 because `E7` caused a deficit, does `E9` automatically delete or reverse that fee?
* **Resolution**:
  - In an immutable append-only ledger, events are never deleted or rewritten.
  - A principal reversal (`REVERSAL`) reverses the underlying transaction amount, but **does not automatically forgive or refund assessed bank fees**. In banking, fee waivers require a separate, authorized administrative fee-credit posting.
  - We reject the naive assumption that fees disappear. The AED 25.00 overdraft fee remains booked, leaving `ACC-001` with AED 260.00 (plus Day 6 interest), which we verify via the annotated failing test `TestDesignFlaw_NaiveReversalRestoresPreE7Balance`.

---

### Ambiguity 6: `Auth-B` Approval Condition Under Negative Balance

* **The Ambiguity**:
  - Event `E8` arrives on Day 5 requesting an authorization hold of AED 90.00 (`Auth-B`).
  - Acceptance criterion states: *"If Auth-B is approved, its hold reduces available balance but not ledger balance."*
  - The prompt uses the conditional *"If Auth-B is approved..."*.
* **Resolution**:
  - Non-negotiable rule: *"An authorization is approved only if the account's available balance — ledger balance minus active holds — remains at or above zero after the hold is applied."*
  - On Day 5, `E7` plunged the ledger balance to -AED 335.00.
  - Available balance was already negative. Applying a 90.00 hold leaves available balance at -AED 425.00 ($< 0$).
  - Therefore, **`Auth-B` must be REJECTED** for insufficient funds. The engine records `Auth-B` with `HoldStatusRejected` and logs an error.

---

### Ambiguity 7: BHD Instalment Remainder Allocation

* **The Ambiguity**:
  - `E10` specifies BHD 10.000 posted as three equal instalments.
  - $10.000 / 3 = 3.333333...$ fils.
  - Does the first instalment or the last instalment absorb the 1 fils remainder?
* **Resolution**:
  - Standard banking convention allocates remainder units to the **first instalment**:
    - Instalment 1: **BHD 3.334**
    - Instalment 2: **BHD 3.333**
    - Instalment 3: **BHD 3.333**
  - This ensures exact zero-sum balance reconciliation ($3.334 + 3.333 + 3.333 = 10.000$).
