import sys
from reportlab.lib.pagesizes import A4
from reportlab.lib import colors
from reportlab.lib.units import mm
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, PageBreak, KeepTogether, HRFlowable
)
from reportlab.pdfgen import canvas

class NumberedCanvas(canvas.Canvas):
    def __init__(self, *args, **kwargs):
        super(NumberedCanvas, self).__init__(*args, **kwargs)
        self._saved_page_states = []

    def showPage(self):
        self._saved_page_states.append(dict(self.__dict__))
        self._startPage()

    def save(self):
        num_pages = len(self._saved_page_states)
        for state in self._saved_page_states:
            self.__dict__.update(state)
            self.draw_page_decorations(num_pages)
            super(NumberedCanvas, self).showPage()
        super(NumberedCanvas, self).save()

    def draw_page_decorations(self, page_count):
        self.saveState()
        self.setFont("Helvetica", 8)
        self.setFillColor(colors.HexColor("#64748b"))
        
        # Header (Pages 2-4)
        if self._pageNumber > 1:
            self.drawString(16 * mm, 285 * mm, "Architecture & Trade-Offs: Core Account Ledger")
            self.drawRightString(194 * mm, 285 * mm, "Production Review | CBUAE Banking")
            self.setStrokeColor(colors.HexColor("#cbd5e1"))
            self.setLineWidth(0.5)
            self.line(16 * mm, 282 * mm, 194 * mm, 282 * mm)

        # Footer (All Pages)
        self.setStrokeColor(colors.HexColor("#cbd5e1"))
        self.setLineWidth(0.5)
        self.line(16 * mm, 14 * mm, 194 * mm, 14 * mm)
        self.drawString(16 * mm, 10 * mm, "CONFIDENTIAL — For Assessment & Review Only")
        page_text = f"Page {self._pageNumber} of {page_count}"
        self.drawRightString(194 * mm, 10 * mm, page_text)
        self.restoreState()

def build_pdf(filename):
    doc = SimpleDocTemplate(
        filename,
        pagesize=A4,
        leftMargin=16 * mm,
        rightMargin=16 * mm,
        topMargin=18 * mm,
        bottomMargin=18 * mm,
    )

    styles = getSampleStyleSheet()

    # Custom styles
    title_style = ParagraphStyle(
        'DocTitle',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=18,
        leading=22,
        textColor=colors.HexColor('#0f172a'),
        spaceAfter=4,
    )

    h1_style = ParagraphStyle(
        'SectionH1',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=12.5,
        leading=16,
        textColor=colors.HexColor('#1e3a8a'),
        spaceBefore=8,
        spaceAfter=6,
        keepWithNext=True,
    )

    h2_style = ParagraphStyle(
        'SectionH2',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=10,
        leading=13,
        textColor=colors.HexColor('#0f172a'),
        spaceBefore=6,
        spaceAfter=3,
        keepWithNext=True,
    )

    body_style = ParagraphStyle(
        'DocBody',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=8.5,
        leading=12,
        textColor=colors.HexColor('#1e293b'),
        spaceAfter=5,
        alignment=4, # Justified
    )

    bullet_style = ParagraphStyle(
        'DocBullet',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=8.5,
        leading=11.5,
        textColor=colors.HexColor('#1e293b'),
        leftIndent=12,
        firstLineIndent=-8,
        spaceAfter=3,
    )

    callout_style = ParagraphStyle(
        'CalloutText',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=8.5,
        leading=12,
        textColor=colors.HexColor('#1e40af'),
        alignment=0,
    )

    table_header_style = ParagraphStyle(
        'TableHeader',
        parent=styles['Normal'],
        fontName='Helvetica-Bold',
        fontSize=7.5,
        leading=9.5,
        textColor=colors.HexColor('#0f172a'),
    )

    table_body_style = ParagraphStyle(
        'TableCell',
        parent=styles['Normal'],
        fontName='Helvetica',
        fontSize=7.5,
        leading=9.5,
        textColor=colors.HexColor('#1e293b'),
    )

    story = []

    # =========================================================================
    # PAGE 1: APPEND-ONLY AT SCALE
    # =========================================================================
    story.append(Paragraph("Architecture & Trade-Offs: Core Account Ledger", title_style))
    story.append(HRFlowable(width="100%", thickness=2, color=colors.HexColor('#2563eb'), spaceBefore=2, spaceAfter=8))

    meta_data = [
        [
            Paragraph("<b>Classification:</b> Production Architecture Review", table_body_style),
            Paragraph("<b>Domain:</b> Core Banking Ledger Engine", table_body_style),
            Paragraph("<b>Regulatory Scope:</b> CBUAE (United Arab Emirates)", table_body_style),
        ]
    ]
    meta_table = Table(meta_data, colWidths=[60*mm, 60*mm, 58*mm])
    meta_table.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,-1), colors.HexColor('#f8fafc')),
        ('BOX', (0,0), (-1,-1), 0.5, colors.HexColor('#cbd5e1')),
        ('VALIGN', (0,0), (-1,-1), 'MIDDLE'),
        ('PADDING', (0,0), (-1,-1), 4),
    ]))
    story.append(meta_table)
    story.append(Spacer(1, 6))

    story.append(Paragraph("1. Append-Only at Scale", h1_style))
    story.append(HRFlowable(width="100%", thickness=0.5, color=colors.HexColor('#cbd5e1'), spaceBefore=1, spaceAfter=6))

    story.append(Paragraph("<b>What Breaks First at 100× Volume?</b>", h2_style))
    story.append(Paragraph(
        "In our core ledger design, balances are modeled across posting phases (<code>COMMITTED</code>, "
        "<code>PENDING_OUTGOING</code>) and stored as point-in-time balance time-series to provide <i>O(1)</i> read performance "
        "for payment authorization and balance inquiry APIs. When transaction velocity scales 100×—from thousands to hundreds of millions "
        "of postings—two structural bottlenecks emerge:", body_style
    ))
    story.append(Paragraph(
        "• <b>Write Amplification on Backdated Postings:</b> When a transaction arrives carrying a past effective value date "
        "(e.g., an automated batch clearing adjustment or retrospective debit), the engine must propagate the delta forward across "
        "all subsequent balance intervals up to the current timestamp. On high-throughput accounts (such as merchant settlement or corporate liquidity "
        "accounts processing 50,000+ daily events), backdating an entry by 5 calendar days forces the ledger to update hundreds of thousands of historical "
        "balance records. At 100× volume, this forward cascade induces severe database lock contention, transaction log saturation, and write amplification.",
        bullet_style
    ))
    story.append(Paragraph(
        "• <b>Read Latency on Un-Checkpointd Replay Streams:</b> If the engine instead re-computes balances on the fly by scanning raw append-only logs, "
        "query latency degrades linearly as <i>O(N)</i>. Core payment gateways and ATM switches enforce strict &lt;50ms response SLAs; "
        "linear historical scans across millions of events quickly exhaust connection pools and trigger upstream timeouts.",
        bullet_style
    ))

    story.append(Paragraph("<b>Where Does the Design Accumulate Unbounded State?</b>", h2_style))
    story.append(Paragraph(
        "The current in-memory engine accumulates state monotonically across three distinct dimensions:", body_style
    ))
    story.append(Paragraph("• <b>Append-Only Event Store:</b> The slice of immutable <code>LedgerEntry</code> records grows indefinitely without automated pruning, tiered archiving, or compaction.", bullet_style))
    story.append(Paragraph("• <b>Dormant Hold Registries:</b> Authorization holds remain indexed in active memory indefinitely even after reaching terminal lifecycle states (<code>SETTLED</code> or <code>REJECTED</code>).", bullet_style))
    story.append(Paragraph("• <b>Intra-Day Balance Time-Series:</b> Storing high-resolution daily closing balance records across multi-year account histories consumes unbounded heap memory.", bullet_style))

    story.append(Paragraph("<b>The Cheapest Structural Change That Defers the Problem</b>", h2_style))
    story.append(Paragraph(
        "The cheapest and most resilient structural intervention is pairing <b>Periodic Balance Checkpointing</b> with an "
        "<b>Enforced Value-Dating Cutoff Window</b>:", body_style
    ))
    story.append(Paragraph(
        "1. <b>EOD & Monthly Checkpointing:</b> At each End-of-Day (EOD) and monthly statement cutoff, the ledger freezes cumulative balances into an immutable checkpoint record. "
        "Historical point-in-time balance queries resolve instantly by reading the nearest checkpoint plus localized intra-day deltas (<i>O(1)</i> read).", bullet_style
    ))
    story.append(Paragraph(
        "2. <b>Enforced Backdating Window (<code>T - k</code>):</b> The ingestion gateway mandates a structural cutoff (e.g., <code>k = 30</code> calendar days). "
        "Postings carrying value dates older than <code>T - k</code> are rejected by automated pipelines and diverted to specialized exception adjustment workflows. "
        "This bounds the maximum forward propagation cascade to at most <code>k</code> days, capping write amplification at an upper bound regardless of how many millions of events exist in cold storage.", bullet_style
    ))

    story.append(PageBreak())

    # =========================================================================
    # PAGE 2: VALUE-DATED ENTRIES IN PRODUCTION
    # =========================================================================
    story.append(Paragraph("2. Value-Dated Entries in Production (UAE-Licensed Bank)", h1_style))
    story.append(HRFlowable(width="100%", thickness=0.5, color=colors.HexColor('#cbd5e1'), spaceBefore=1, spaceAfter=8))

    story.append(Paragraph("<b>Operational and Regulatory Surface in a CBUAE Environment</b>", h2_style))
    story.append(Paragraph(
        "Within the regulatory jurisdiction of the Central Bank of the UAE (CBUAE), value-dated entries introduce critical regulatory exposure, "
        "liquidity risk, and consumer protection friction:", body_style
    ))
    story.append(Paragraph(
        "• <b>Prudential Liquidity & Daily Regulatory Returns:</b> Under CBUAE Regulations Regarding Liquidity Risk (Circular No. 33/2015), licensed commercial banks "
        "must compute and submit daily returns for the <b>Liquidity Coverage Ratio (LCR)</b> and the <b>Eligible Liquid Assets Ratio (ELAR)</b>. "
        "Value-dated debits retroactively decrease customer deposit liabilities. If a corporate debit of AED 50,000,000 is backdated by 3 business days, "
        "the bank's previously certified regulatory filings for Days <code>T-3</code>, <code>T-2</code>, and <code>T-1</code> were retroactively inaccurate. "
        "Such discrepancies distort the bank's High-Quality Liquid Assets (HQLA) baseline and expose the institution to regulatory audits, mandatory capital surcharges, "
        "and supervisory penalties.", bullet_style
    ))
    story.append(Paragraph(
        "• <b>Customer Statement Truth & Consumer Protection:</b> The CBUAE Consumer Protection Regulation (Circular No. 8/2020) mandates that customer account statements "
        "be transparent, finalized, and legally binding. Injecting a backdated posting into a closed monthly accounting period causes the customer's legal statement "
        "to diverge from the core ledger. In the UAE retail sector, this triggers formal complaints to <b>Sanadak</b> (the Ombudsman Unit for Financial and Insurance Disputes), "
        "resulting in dispute arbitration penalties, mandatory customer compensation, and reputational harm.", bullet_style
    ))
    story.append(Paragraph(
        "• <b>Retroactive Overdraft Fees & Sharia Compliance:</b> In Islamic banking windows operating under Higher Sharia Authority (HSA) guidelines, assessing retroactive penalties "
        "or compensatory fees on historical deficits is strictly non-compliant with Islamic finance principles (prohibition of Riba). In conventional banking, penalizing retail consumers "
        "retroactively for an unarranged overdraft that was impossible to observe or mitigate in real time breaches CBUAE consumer fairness standards.", bullet_style
    ))

    story.append(Spacer(1, 4))
    story.append(Paragraph("<b>The One Control to Add Before Going Live</b>", h2_style))

    callout_data = [[
        Paragraph("<b>Mandatory Control:</b> Pre-Settlement Value-Dating Boundary with Dual-Authorization (Maker-Checker) Quarantine Workflow", callout_style)
    ]]
    callout_table = Table(callout_data, colWidths=[178*mm])
    callout_table.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,-1), colors.HexColor('#eff6ff')),
        ('BOX', (0,0), (-1,-1), 1, colors.HexColor('#bfdbfe')),
        ('LINELEFT', (0,0), (-1,-1), 3.5, colors.HexColor('#2563eb')),
        ('PADDING', (0,0), (-1,-1), 6),
    ]))
    story.append(callout_table)
    story.append(Spacer(1, 4))

    story.append(Paragraph(
        "Before promoting the ledger to production, a strict <b>Value-Date Quarantine Gateway</b> must be enforced at the ledger ingestion boundary:", body_style
    ))
    story.append(Paragraph(
        "• <b>Automated Processing Threshold (<code>T - 2</code>):</b> Automated clearing channels (UAE Funds Transfer System - UAEFTS, Instant Payments Platform - Aani, "
        "Image Cheque Clearing System, and Visa/Mastercard clearing files) are permitted an automated backdating tolerance of at most 2 business days.", bullet_style
    ))
    story.append(Paragraph(
        "• <b>Quarantine Interceptor (&gt; <code>T - 2</code> or Closed Cycles):</b> Any inbound posting instruction referencing a value date older than 2 business days—or falling "
        "into a previously finalized monthly statement period—is automatically intercepted by the ingestion pipeline and placed in a quarantined suspense ledger.", bullet_style
    ))
    story.append(Paragraph(
        "• <b>Maker-Checker Operational Approval:</b> Quarantined postings cannot post directly to customer liability accounts. A Financial Operations Officer (Maker) and a "
        "Compliance Officer (Checker) must explicitly review the retroactive liquidity impact on daily LCR metrics and sign off on an authorized General Ledger compensatory booking.", bullet_style
    ))

    story.append(PageBreak())

    # =========================================================================
    # PAGE 3: AUTHORIZATION LIFECYCLE
    # =========================================================================
    story.append(Paragraph("3. Authorization Lifecycle", h1_style))
    story.append(HRFlowable(width="100%", thickness=0.5, color=colors.HexColor('#cbd5e1'), spaceBefore=1, spaceAfter=8))

    story.append(Paragraph(
        "In our posting model, an authorization reserves purchasing power by recording an allocation in the <code>PENDING_OUTGOING</code> phase without debiting "
        "the <code>COMMITTED</code> ledger balance. While a standard transaction ends with a matching settlement, production banking mandates strict handling for "
        "five alternative termination lifecycles:", body_style
    ))

    story.append(Paragraph("<b>1. TTL Expiration (Time-To-Live Timeout)</b>", h2_style))
    story.append(Paragraph(
        "• <i>Real-World Scenario:</i> A customer checks into a hotel or rents a vehicle, and a pre-authorization hold of AED 1,000.00 is placed on their card. "
        "At checkout, the customer pays the final bill using cash or an alternative corporate card. The merchant never transmits an explicit cancellation message.", bullet_style
    ))
    story.append(Paragraph(
        "• <i>Mandated System Behavior:</i> An automated EOD daemon scans all active authorizations. If a hold remains in <code>PENDING_OUTGOING</code> past its scheme-mandated TTL "
        "(e.g., 7 days for retail POS, 30 days for vehicle rental/hotel), the system transitions the hold to <code>EXPIRED</code>, deducts the hold amount from <code>PENDING_OUTGOING</code>, "
        "and immediately restores customer <code>AvailableBalance</code>.", bullet_style
    ))

    story.append(Paragraph("<b>2. Merchant Reversal / POS Void</b>", h2_style))
    story.append(Paragraph(
        "• <i>Real-World Scenario:</i> An automated fuel dispenser pre-authorizes AED 200.00. The pump handle is hung up immediately without dispensing fuel, or a grocery store "
        "cashier voids a purchase at the checkout register. The terminal generates an immediate ISO 8583 reversal notification (0400/0420).", bullet_style
    ))
    story.append(Paragraph(
        "• <i>Mandated System Behavior:</i> The engine matches the incoming reversal against the original <code>AuthID</code>, transitions the hold status to <code>RELEASED</code>, "
        "and decrements <code>PENDING_OUTGOING</code> in real time (&lt;100ms), restoring purchasing power with zero impact on <code>COMMITTED</code> ledger balance.", bullet_style
    ))

    story.append(Paragraph("<b>3. Partial Settlement with Residual Release</b>", h2_style))
    story.append(Paragraph(
        "• <i>Real-World Scenario:</i> An e-commerce merchant pre-authorizes an order for AED 500.00. One item priced at AED 150.00 is out of stock. The merchant ships the available "
        "items and transmits a final clearing settlement presentment of AED 350.00 with the <code>final_presentment</code> flag enabled.", bullet_style
    ))
    story.append(Paragraph(
        "• <i>Mandated System Behavior:</i> The engine debits AED 350.00 to <code>COMMITTED</code>, marks the authorization hold as <code>SETTLED</code>, and automatically releases "
        "the residual AED 150.00 hold from <code>PENDING_OUTGOING</code>, preventing customer funds from remaining locked.", bullet_style
    ))

    story.append(Paragraph("<b>4. Incremental Authorization (Hold Upsizing)</b>", h2_style))
    story.append(Paragraph(
        "• <i>Real-World Scenario:</i> A customer at a restaurant authorizes AED 250.00 for a meal and subsequently adds an AED 50.00 gratuity at checkout, or a hotel guest extends "
        "their reservation by two additional nights.", bullet_style
    ))
    story.append(Paragraph(
        "• <i>Mandated System Behavior:</i> Rather than spawning an unlinked secondary authorization, the engine accepts an incremental authorization instruction referencing the active "
        "<code>AuthID</code>. The engine evaluates: <code>AvailableBalance &gt;= incremental_amount</code>. If approved, <code>PENDING_OUTGOING</code> increases from AED 250.00 to AED 300.00 "
        "under the same hold entity.", bullet_style
    ))

    story.append(Paragraph("<b>5. Administrative / Anti-Fraud Termination</b>", h2_style))
    story.append(Paragraph(
        "• <i>Real-World Scenario:</i> The bank’s real-time fraud monitoring engine detects that a compromised card credential was utilized in an unauthorized overseas transaction, "
        "placing an active authorization hold minutes earlier.", bullet_style
    ))
    story.append(Paragraph(
        "• <i>Mandated System Behavior:</i> Fraud Operations triggers an emergency administrative termination. The engine transitions the hold to <code>BLOCKED_FRAUD</code>, removes the hold "
        "from <code>PENDING_OUTGOING</code>, freezes the payment token, and routes any subsequent settlement presentment into dispute chargeback queues.", bullet_style
    ))

    story.append(PageBreak())

    # =========================================================================
    # PAGE 4: WHAT WAS CUT AND WHY
    # =========================================================================
    story.append(Paragraph("4. What Was Cut and Why", h1_style))
    story.append(HRFlowable(width="100%", thickness=0.5, color=colors.HexColor('#cbd5e1'), spaceBefore=1, spaceAfter=8))

    story.append(Paragraph(
        "To deliver a verifiable, production-aligned core ledger engine within scope, deliberate simplifications were enacted. "
        "Every cut represents a conscious engineering trade-off that defers specific operational risks:", body_style
    ))

    table_data = [
        [
            Paragraph("<b>Simplified Component</b>", table_header_style),
            Paragraph("<b>In-Memory Implementation</b>", table_header_style),
            Paragraph("<b>Production Target</b>", table_header_style),
            Paragraph("<b>Deferred Production Risk</b>", table_header_style),
        ],
        [
            Paragraph("<b>Consensus & Storage</b>", table_body_style),
            Paragraph("Single-node append-only memory slice", table_body_style),
            Paragraph("Distributed commit log (Kafka) + Spanner / CockroachDB", table_body_style),
            Paragraph("Defers distributed split-brain recovery, network partitions, 2-phase commit (2PC) failures, and Raft log compaction overhead.", table_body_style),
        ],
        [
            Paragraph("<b>EOD Scheduling</b>", table_body_style),
            Paragraph("Synchronous replay loop invocation", table_body_style),
            Paragraph("Asynchronous distributed workflow (Temporal / Cloud Scheduler)", table_body_style),
            Paragraph("Defers job executor heartbeats, missed midnight cron triggers, UTC vs. GST timezone drift, and distributed retry idempotency.", table_body_style),
        ],
        [
            Paragraph("<b>Accounting Integrity</b>", table_body_style),
            Paragraph("Single-entry customer deposit liabilities", table_body_style),
            Paragraph("Balanced multi-legged General Ledger (A = L + E)", table_body_style),
            Paragraph("Defers balancing institutional GL accounts (Due from Central Bank, Interchange Clearing, Fee Income) and GL reconciliation loops.", table_body_style),
        ],
        [
            Paragraph("<b>Concurrency Control</b>", table_body_style),
            Paragraph("Deterministic sequential stream processing", table_body_style),
            Paragraph("Optimistic Concurrency Control (OCC) / Row locks", table_body_style),
            Paragraph("Defers database lock contention, transaction deadlocks, and OCC serialization retry overhead under multi-threaded load.", table_body_style),
        ],
        [
            Paragraph("<b>Multi-Currency</b>", table_body_style),
            Paragraph("Isolated precision (AED: 2 decimals, BHD: 3 decimals)", table_body_style),
            Paragraph("Real-time FX treasury engine with live rate feeds", table_body_style),
            Paragraph("Defers real-time exchange rate slippage, cross-currency settlement risk, and intra-day foreign exchange revaluations.", table_body_style),
        ],
    ]

    tradeoff_table = Table(table_data, colWidths=[38*mm, 42*mm, 48*mm, 50*mm])
    tradeoff_table.setStyle(TableStyle([
        ('BACKGROUND', (0,0), (-1,0), colors.HexColor('#f1f5f9')),
        ('BOX', (0,0), (-1,-1), 0.5, colors.HexColor('#cbd5e1')),
        ('INNERGRID', (0,0), (-1,-1), 0.5, colors.HexColor('#cbd5e1')),
        ('VALIGN', (0,0), (-1,-1), 'TOP'),
        ('PADDING', (0,0), (-1,-1), 4),
        ('ROWBACKGROUNDS', (0,1), (-1,-1), [colors.white, colors.HexColor('#f8fafc')]),
    ]))
    story.append(tradeoff_table)
    story.append(Spacer(1, 8))

    story.append(Paragraph("<b>Concurrency Vulnerability Analysis (Design Flaw Test Correlation)</b>", h2_style))
    story.append(Paragraph(
        "As documented in our intentional failing test <code>TestDesignFlaw_ConcurrentAuthorizationRaceBreachesAvailableBalance</code>, "
        "sequential in-memory execution defers thread synchronization primitives. Under high-throughput multi-threaded ingestion:", body_style
    ))
    story.append(Paragraph(
        "• An account begins with an available balance of <b>AED 200.00</b>.<br/>"
        "• Two concurrent authorization threads each request a hold of <b>AED 150.00</b>.<br/>"
        "• Both threads evaluate <code>GetAvailableBalance()</code> concurrently, both observe <code>200.00 &gt;= 150.00</code>, both approve the hold, "
        "and the account’s available balance plunges to <b>-AED 100.00</b>.", bullet_style
    ))
    story.append(Paragraph(
        "This breaches the non-negotiable core invariant: <i>'An authorization is approved only if available balance remains at or above zero.'</i><br/>"
        "<b>Production Mitigation:</b> In a live distributed banking platform, this risk is mitigated by partitioning accounts across single-threaded deterministic actor queues "
        "or by enforcing <b>Optimistic Concurrency Control (OCC)</b> with account version stamps (<code>version_id</code>), ensuring concurrent mutations abort and retry cleanly.",
        body_style
    ))

    doc.build(story, canvasmaker=NumberedCanvas)
    print(f"Successfully generated {filename}")

if __name__ == '__main__':
    output_path = "architecture_and_trade_offs_doc/ARCHITECTURE_AND_TRADE_OFFS.pdf"
    if len(sys.argv) > 1:
        output_path = sys.argv[1]
    build_pdf(output_path)
