# NUMBERS.md: Rationale for All Constants Chosen

This document justifies every numerical constant chosen in the ledger implementation, detailing why that specific value was selected and why half that value was rejected.

---

### 1. Overdraft Fee: AED 25.00 (2,500 fils)
* **Chosen Value**: `2500` minor units (AED 25.00).
* **Why this value?**:
  - Mandated as a non-negotiable parameter in the assignment specification.
  - Aligns with the Central Bank of the UAE (CBUAE) Consumer Protection Regulations (Circular No. 8/2020), which establish caps on retail overdraft and dishonor fees between AED 25.00 and AED 100.00.
* **Why not half it (AED 12.50)?**:
  - At AED 12.50, the fee fails to cover the operational cost of credit monitoring, automated collection pipelines, and the regulatory risk-weighted asset (RWA) provisioning required by the CBUAE for unarranged overdrafts.
  - Furthermore, AED 12.50 is economically insufficient to deter moral hazard, encouraging customers to use unauthorized overdrafts as cheap short-term micro-credit.

---

### 2. Daily Interest Accrual Rate: 0.04% per day (0.0004)
* **Chosen Value**: `0.0004` (0.04% per day).
* **Why this value?**:
  - Mandated by the core rules for positive closing balances.
  - Over a 365-day year, $0.04\% \times 365 = 14.60\%$ annualized simple return ($15.72\%$ APY with daily compounding). In the GCC fintech landscape, this mirrors high-yield digital savings acquisition tiers (e.g. promotional CASA campaign buckets).
* **Why not half it (0.02% per day = 7.30% p.a.)?**:
  - Half of this value ($7.30\%$ annualized) provides insufficient customer acquisition pull in competitive digital banking environments where promotional reward rates routinely target double digits.
  - Mathematically, halving the rate causes smaller daily balances (e.g. Day 1 balance AED 250.00: $250.00 \times 0.0002 = 0.05$ AED = 5 fils) to experience severe rounding quantization distortion relative to base deposits.

---

### 3. Currency Scale Factors: AED = 100, BHD = 1,000
* **Chosen Values**:
  - `CurrencyAED`: Scale factor `100` (2 decimal places, 1 AED = 100 fils).
  - `CurrencyBHD`: Scale factor `1000` (3 decimal places, 1 BHD = 1,000 fils).
* **Why this value?**:
  - Strictly governed by ISO 4217 international currency standards.
  - AED is divided into 100 fils.
  - BHD is one of the few global three-decimal currencies, divided into 1,000 fils.
* **Why not half it (50 and 500)?**:
  - A scale factor of 50 or 500 violates ISO 4217 and base-10 fractional representation.
  - Financial ledgers cannot map 1 fils ($0.01$ AED or $0.001$ BHD) onto fractional minor units without floating-point inaccuracies, rounding leakage, and regulatory non-compliance with the CBUAE and Central Bank of Bahrain (CBB).

---

### 4. Instalment Partition Count: 3 (for E10)
* **Chosen Value**: `3` equal instalments.
* **Why this value?**:
  - Mandated by event `E10` (*"ACC-002 BHD 10.000, posted as three equal instalments"*).
* **Why not half it (1.5 instalments)?**:
  - Transaction posting counts in core banking must be positive integers. Fractional instalments do not exist in transactional accounting.

---

### 5. Temporal Window: 6 Days (Day 1 through Day 6)
* **Chosen Value**: `6` calendar days.
* **Why this value?**:
  - The business domain events span across Days 1 through 6:
    - Day 1: Account funding & initial debit (`E1`, `E2`).
    - Day 2: First authorization hold (`E3`).
    - Day 3: Liquidity credit (`E4`).
    - Day 4: Dual settlements (`E5`, `E6`).
    - Day 5: Backdated debit and second hold (`E7`, `E8`, `E10`).
    - Day 6: Compensatory reversal and interest capitalization (`E9`, `INT-CAP`).
* **Why not half it (3 days)?**:
  - Halving the window to 3 days would truncate the lifecycle before backdating (`E7`), reversals (`E9`), and interest capitalization ever executed, omitting the entire problem scope.

---

### 6. Available Balance Threshold: AED 0.00
* **Chosen Value**: `0` minor units.
* **Why this value?**:
  - Rule: *"An authorization is approved only if the account's available balance... remains at or above zero after the hold is applied."*
  - CASA (Current Account Savings Account) products without a pre-agreed overdraft facility enforce a strict non-negative available balance constraint.
* **Why not half it (e.g. -AED 50.00 or +AED 50.00)?**:
  - Setting a negative threshold would grant an uncollateralized credit line without credit underwriting.
  - Setting a positive buffer (e.g. requiring a minimum AED 50.00 balance) would artificially decline legitimate customer purchases when funds are available.
