# Architecture & Trade-Offs: Core Account Ledger

**Document Classification:** Production Architectural Review  
**Domain:** Core Banking & Transaction Processing  
**Target Platform:** UAE-Licensed Retail & Commercial Banking  

---

## 1. Append-Only at Scale

### What Breaks First at 100× Volume?
In our core ledger design, monetary balances are stored as point-in-time time-series records across posting dimensions (`COMMITTED`, `PENDING_OUTGOING`) to provide $O(1)$ read performance for authorization and balance inquiries. When volume increases 100× (scaling from thousands to hundreds of millions of postings), two structural bottlenecks emerge:

1. **Write Amplification on Backdated Postings**:
   When an event with a past value date (such as a backdated clearing adjustment) arrives, the engine must propagate the delta forward across all subsequent balance intervals up to the current timestamp. On an account with high transaction velocity (e.g., a corporate treasury or merchant acquiring settlement account with 50,000 daily events), inserting an entry backdated by 5 days requires updating or splitting hundreds of thousands of historical balance intervals. At 100× volume, this forward-cascading update induces severe database lock contention, transaction log saturation, and write amplification.
2. **Read Latency on Un-Checkpointd Event Streams**:
   If the ledger instead relies on replaying raw append-only event logs to derive balances dynamically, query latency scales linearly as $O(N)$. High-frequency authorization checks (which operate under strict <50ms SLAs) degrade rapidly as account history deepens, ultimately exhausting worker connection pools.

### Where Does the Design Accumulate Unbounded State?
The current implementation retains all historical ledger entries, balance records, and authorization holds in active memory:
- **Append-Only Log**: The slice of `LedgerEntry` records grows monotonically without pruning or cold storage archival.
- **Hold Registry**: Authorization holds remain in memory indefinitely even after transitioning to `SETTLED` or `REJECTED`.
- **Daily Snapshots**: Storing daily balance records across years of activity accumulates linear memory overhead per account.

### The Cheapest Structural Change That Defers the Problem
The cheapest and most resilient structural intervention is the combination of **Periodic Balance Checkpointing** with an **Enforced Value-Dating Cutoff Window**:

```
[Raw Event Log (Cold Storage)]
               │
               ▼
[Monthly/EOD Checkpoint Snapshot] ───► [Bounded Dynamic Window (T - 30 Days)] ───► Live Balance O(1)
```

1. **Daily & Monthly Checkpoints**: At each End-of-Day (EOD) and monthly statement cutoff, the ledger generates an immutable balance snapshot. Historical point-in-time queries resolve against the nearest checkpoint plus localized intra-day deltas ($O(1)$ read).
2. **Enforced Backdating Window ($T - k$)**: The engine imposes a hard boundary (e.g., $k = 30$ calendar days). Any incoming transaction with a value date older than $T - k$ is rejected by automated posting pipelines and diverted to an exception queue. This bounds the maximum forward propagation cascade to at most $k$ intervals, capping write amplification at a fixed upper bound regardless of account age.

---

## 2. Value-Dated Entries in Production (UAE-Licensed Bank)

### Operational and Regulatory Surface
In a financial institution regulated by the Central Bank of the UAE (CBUAE), value-dated entries introduce significant operational fragility, financial reporting distortions, and compliance liabilities:

1. **Prudential Liquidity & Daily Regulatory Reporting**:
   Under CBUAE Regulations Regarding Liquidity Risk (Circular No. 33/2015), licensed banks must monitor and submit daily returns for the **Liquidity Coverage Ratio (LCR)** and the **Eligible Liquid Assets Ratio (ELAR)**. Value-dated debits retroactively decrease the bank's historical customer deposit liabilities. If an entry dated $T-3$ reduces a corporate deposit by AED 50 million, the bank’s previously filed regulatory liquidity position for Days $T-3$, $T-2$, and $T-1$ was overstated. Retroactive recalculations can trigger regulatory inquiries, capital adequacy penalties, and mandatory restatements to the central bank.
2. **Consumer Protection & Customer Statement Truth**:
   The CBUAE Consumer Protection Regulation (Circular No. 8/2020) mandates that customer account statements reflect accurate, non-misleading, and final historical balances. If a value-dated transaction is injected into a closed billing period for which a customer has already received an official monthly statement, the ledger’s balance diverges from the customer's legal document. This leads to customer dispute escalations through **Sanadak** (the UAE Ombudsman Unit for Financial and Insurance Disputes), exposing the bank to administrative fines.
3. **Retroactive Overdraft Fees & Sharia Governance**:
   For Islamic banking windows operating under the governance of the Higher Sharia Authority (HSA), assessing retroactive overdraft penalties on backdated negative balances is strictly non-compliant with Murabaha/Qard principles. In conventional banking, penalizing a customer retroactively for a negative balance they could not observe or prevent in real time breaches CBUAE consumer fairness doctrines.

### The One Control to Add Before Going Live
**Pre-Settlement Value-Dating Boundary with Dual-Authorization (Maker-Checker) Quarantine**:

Before promoting the ledger to production, implement a hard **Value-Date Interceptor Gateway**:
- **Automated Processing Threshold ($T-2$)**: Clearing networks (UAE FTS, Image Cheque Clearing System, and card schemes) are granted an automated backdating window of at most 2 business days.
- **Quarantine Interceptor (> $T-2$ or Closed Periods)**: Any transaction carrying a value date exceeding 2 business days in the past—or referencing a finalized accounting/statement cycle—is automatically paused and assigned to a quarantine ledger.
- **Maker-Checker Release**: The transaction cannot post to customer-facing accounts until the Bank Operations Lead (Maker) and Compliance Officer (Checker) review the regulatory impact on daily LCR and execute an explicit compensatory general ledger booking.

---

## 3. Authorization Lifecycle

In our posting model, an authorization reserves purchasing power by recording an allocation in the `PENDING_OUTGOING` phase without debiting the `COMMITTED` ledger balance. Beyond a standard matching settlement, an authorization can terminate in five operational scenarios:

```
                      ┌───────────────► SETTLED (Matching Settlement)
                      │
                      ├───────────────► EXPIRED (TTL Timeout Sweep)
                      │
   AUTHORIZATION ─────┼───────────────► VOIDED (Merchant POS Cancellation)
  (PENDING_OUTGOING)  │
                      ├───────────────► PARTIALLY_SETTLED (Residual Release)
                      │
                      ├───────────────► INCREMENTAL (Hold Upsizing)
                      │
                      └───────────────► BLOCKED_FRAUD (Risk Intervention)
```

### 1. TTL Expiration (Time-To-Live Timeout)
* **Real-World Scenario**: A customer provides a security deposit authorization at a hotel or car rental merchant. Upon checkout, the customer settles the bill using a corporate card or cash. The merchant never issues a release instruction, leaving the original card authorization dormant.
* **Mandated System Behavior**: An automated background job sweeps authorizations nightly. Any hold remaining in `PENDING_OUTGOING` past its scheme-mandated TTL (e.g., 7 days for retail POS, 30 days for vehicle rental) is transitioned to `EXPIRED`. The hold amount is deducted from `PENDING_OUTGOING`, immediately returning the funds to `AvailableBalance`.

### 2. Merchant Reversal / POS Void
* **Real-World Scenario**: An automated fuel dispenser pre-authorizes AED 150.00. The pump handle is replaced immediately without dispensing fuel, or a supermarket cashier cancels a transaction at the terminal. The point-of-sale generates an immediate ISO 8583 reversal message (0400/0420).
* **Mandated System Behavior**: The engine matches the incoming reversal against the authorization ID, transitions the hold status to `RELEASED`, and decrements `PENDING_OUTGOING` in real time (<100ms), restoring available purchasing power without booking any entry to `COMMITTED`.

### 3. Partial Settlement with Residual Release
* **Real-World Scenario**: An online retailer authorizes an order for AED 600.00. One item priced at AED 200.00 is out of stock. The merchant ships the available items and presents a clearing settlement for AED 400.00, accompanied by a final presentment indicator.
* **Mandated System Behavior**: The engine books a debit of AED 400.00 to `COMMITTED`, marks the authorization as `SETTLED`, and releases the remaining AED 200.00 from `PENDING_OUTGOING`. The residual funds immediately return to available balance, preventing funds from remaining locked.

### 4. Incremental Authorization (Hold Upsizing)
* **Real-World Scenario**: A restaurant guest authorizes an initial bill of AED 300.00. Before final settlement, the guest adds an AED 50.00 gratuity, or a hotel guest extends their stay by an additional evening.
* **Mandated System Behavior**: Rather than spawning an unlinked secondary authorization, the engine receives an incremental authorization request tied to the existing `AuthID`. It evaluates whether `AvailableBalance >= additional_hold`. Upon approval, `PENDING_OUTGOING` is increased from 300.00 to 350.00 under the same hold lifecycle identifier.

### 5. Administrative / Anti-Fraud Termination
* **Real-World Scenario**: The bank's machine learning fraud detection system detects that a customer's payment credential was compromised in a credential-stuffing attack, and unauthorized authorizations were generated moments earlier.
* **Mandated System Behavior**: Fraud operations issues an administrative hold kill instruction. The engine transitions the authorization to `BLOCKED_FRAUD`, removes the hold from `PENDING_OUTGOING`, locks the underlying payment token, and routes any subsequent settlement presentment to the dispute and chargeback clearing queue.

---

## 4. What Was Cut and Why

To deliver a verifiable, production-aligned core ledger engine within scope, deliberate simplifications were made. Each cut represents a conscious trade-off that defers specific operational risks:

| Simplified Component | Implementation in Core | Production Alternative | Deferred Production Risk |
| :--- | :--- | :--- | :--- |
| **Consensus & Storage** | In-memory append-only array | Distributed commit log (Kafka) + Spanner/CockroachDB | Defers distributed split-brain handling, partition recovery, 2-phase commit (2PC) failures, and Raft log compaction overhead. |
| **EOD Scheduling** | Synchronous replay loop invocation | Asynchronous distributed workflow (Temporal / Cloud Scheduler) | Defers job executor heartbeat failures, missed midnight cron triggers, timezone boundary drift (UTC vs. GST), and distributed retry idempotency. |
| **Accounting Legs** | Single-entry customer liability accounts | Full double-entry balanced chart of accounts ($A = L + E$) | Defers balancing institutional general ledger accounts (e.g. offsetting Customer Deposits with Due from Central Bank, Interchange Settlement, and Fee Income). |
| **Concurrency Control** | Deterministic sequential execution | Optimistic Concurrency Control (OCC) / Row-level locking | Defers lock contention, deadlocks, and distributed lock timeouts under multi-threaded high-concurrency ingestion. |
| **Multi-Currency** | Isolated currency precision (AED/BHD) | Dynamic FX conversion engine with real-time treasury rates | Defers managing real-time currency conversion slippage, cross-currency settlement risk, and intra-day foreign exchange revaluation. |

---

### Concurrency Vulnerability Analysis (Annotated Test Correlation)
As proven by our failing test `TestDesignFlaw_ConcurrentAuthorizationRaceBreachesAvailableBalance`, single-threaded in-memory execution defers thread synchronization. If two authorization requests execute concurrently against an account with AED 200.00 available balance, both read the available balance simultaneously, both approve, and the account overdraws to -AED 100.00. 

In production, this risk is mitigated by partitioning accounts across deterministic actor threads or enforcing Optimistic Concurrency Control with version stamping (`account_version`) in the database tier.
