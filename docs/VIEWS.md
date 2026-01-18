# Dashboard Views & UX Strategy

## 1. Cost Allocation View ("The Financial Truth")
**Goal**: Provide clear visibility into who is spending what, resolving the "Shared Bill" conflict.

### 🧠 The Pain Point
The #1 frustration in Kubernetes isn't technical—it's political. *“Who is responsible for this bill?”*
Organizations run multi-tenant clusters. When the cloud bill arrives, it's a massive, unassigned sum. FinOps teams need to accurately chargeback costs to specific teams (e.g., "Frontend" vs. "Data Science"). Without this clarity, the product provides no value to management.

### 🚶 The User Journey
1.  **Initial State**: The user sees a large aggregate number: **"projected Monthly Spend: $5,200"**.
2.  **The Question**: Immediately asks, *"Why is it so high?"*.
3.  **Discovery**: Scrolls down to see a breakdown by **Namespace** or **Label**.
4.  **Insight**: Identifies that the `ai-training` namespace consumes **60%** of the budget.
5.  **Action**: Clicks for details and shares a permalink report with that team.

### 🎨 UX & Visuals
*   **Main Visual**: **Sunburst Chart** or **Treemap**. These effectively visualize hierarchy (Cluster → Namespace → Workload).
*   **Interaction**: Hovering over a slice dims the rest and highlights specific costs with a floating tooltip.
*   **Differentiator**: **"Unit of Economics" Selector**. Allow users to toggle between:
    *   Currency ($)
    *   Carbon Footprint (CO₂e)
    *   % of Budget
*   **Key Features**:
    *   **Label Mapping**: Group costs by custom business labels (e.g., `owner: jesus`, `team: platform`).
    *   **Idle Cost Allocation**: Option to distribute "Idle/Unused" cluster costs proportionally across tenants.

---

## 2. Optimization View ("The Right-Sizing Engine")
**Goal**: Build confidence to reduce resource request limits without fearing stability issues.

### 🧠 The Pain Point
**Fear**. Engineers set high requests (e.g., 4 CPU) to prevent crashes, even if the app uses only 0.2 CPU. They are terrified of OOMKilled errors. The tool must sell **confidence**, not just data.

### 🚶 The User Journey
1.  **Entry**: User clicks the **"Savings"** tab.
2.  **Overview**: Sees a table sorted by **"Wasted Spend"**.
3.  **Detail**: Sees their main deployment highlighted in red.
4.  **Analysis**: Expands the row to see a line chart:
    *   **Grey Line**: Configured Limit (High).
    *   **Green Line**: Actual P99 Usage (Low).
5.  **Recommendation**: System advises: *"You can safely reduce CPU to 0.5 with Low Risk."*
6.  **Action**: Copies the suggested YAML snippet or clicks "Apply" (if GitOps integrated).

### 🎨 UX & Visuals
*   **Main Visual**: **Bullet Charts** (Overlapping progress bars).
    *   **Grey Background**: Request (Cost).
    *   **Green Bar**: Usage P99 (Real Needs).
    *   **Empty Space**: Waste.
*   **Risk Semaphores**: Every recommendation must be tagged:
    *   🟢 **Safe**: < 50% usage peaks over 30 days.
    *   🟡 **Moderate**: Occasional spikes detected.
*   **Action**: A slide-out **Drawer** showing the Diff (Before vs. After).

---

## 3. Network Radar View ("The Invisible Cost")
**Goal**: Visualize detailed network flows and their associated costs (Egress vs. Cross-AZ).

### 🧠 The Pain Point
This is where **eBPF** shines. Network transfer costs are notoriously opaque. AWS charges for:
1.  **Internet Egress**.
2.  **Cross-AZ Traffic**. (Common misconfiguration: Pod A in Zone 1 talking to Pod B in Zone 2).

### 🚶 The User Journey
1.  **Entry**: User selects **"Network Costs"**.
2.  **Visualization**: Sees the topology map.
3.  **Filter**: Toggles **"Show Money Flows"**.
4.  **Insight**: The map dims; expensive links (Internet/Cross-AZ) glow **Neon Orange**.
5.  **Discovery**: Clicks a thick line routing to "Internet" and discovers a log-shipper sending TBs to an external IP.

### 🎨 UX & Visuals
*   **Main Visual**: **Force-Directed Graph** (Topology).
*   **Hierarchy**:
    *   **Cloud (Top)**: Nodes grouped by external provider (S3, Google API, Auth0).
    *   **Cluster (Bottom)**: Internal services.
*   **Edges**:
    *   **Thickness**: Cost ($).
    *   **Color**: Traffic Type (🔴 Internet, 🟡 Cross-AZ, ⚪ Local).
*   **Unique Feature**: **"NAT Hairpinning" Detection**. Flag traffic that exits and re-enters the cluster as a critical waste.

---

## 4. Infrastructure Health View ("The Node Tetris")
**Goal**: Optimize cluster density and bin-packing.

### 🧠 The Pain Point
**Bin Packing**. Paying for a whole Box (Node) to ship one Book (Pod). Users want to know if they can turn off nodes to save money but struggle with the "Tetris" of reorganizing pods.

### 🚶 The User Journey
1.  **Entry**: User navigates to **"Infrastructure"**.
2.  **Observation**: Sees 10 nodes; 3 look visually empty.
3.  **Suggestion**: System alerts: **"Consolidation Possible"**.
4.  **Simulation**: *"Move pods from Node 9 & 10 to others to terminate 2 nodes and save $400/mo."*

### 🎨 UX & Visuals
*   **Main Visual**: **Waffle Charts** or **Rectangular Map** per node.
    *   **Big Rectangle**: Node.
    *   **Small Blocks**: Pods.
*   **Color Coding**:
    *   ⬜ **White Space**: Real free capacity.
    *   🌫 **Grey Blocks**: Reserved but unused capacity (Slack).
    *   🟦 **Colored Blocks**: Actual Application Usage.
*   **KPI**: **Cluster Density Score**. *"Your cluster is 45% dense. Goal: 75%."*
