# Monitoring dashboard design QA

- Source visual truth: `C:\Users\ADMINI~1\AppData\Local\Temp\codex-clipboard-d50340f0-0d41-4977-90f3-cb4083226576.png`
- Issue reference — duplicated disk filesystems: `C:\Users\ADMINI~1\AppData\Local\Temp\codex-clipboard-a180c48c-4a0a-4bfa-a2bb-806cb120ab08.png`
- Issue reference — requested panel ordering: `C:\Users\ADMINI~1\AppData\Local\Temp\codex-clipboard-ff9d7c4c-142b-4735-ae56-fb6115a072d4.png`
- Implementation route: `http://localhost:8080/monitor/dashboards`
- Implementation screenshot: `D:\go\ops-admin\design-qa-assets\monitor-dashboard-disk-memory-final-2.png`
- Full-page implementation screenshot: `D:\go\ops-admin\design-qa-assets\monitor-dashboard-three-column-full.png`
- Responsive screenshot: `D:\go\ops-admin\design-qa-assets\monitor-dashboard-responsive.png`
- K8s source reference: `C:\Users\ADMINI~1\AppData\Local\Temp\codex-clipboard-48940c12-c24b-48c3-83d9-639bb5799137.png`
- K8s Pod overview screenshot: `D:\go\ops-admin\design-qa-assets\monitor-dashboard-k8s-pod-final.png`
- K8s Pod detail screenshot: `D:\go\ops-admin\design-qa-assets\monitor-dashboard-k8s-pod-detail-final.png`
- Revised K8s resource screenshot: `D:\go\ops-admin\design-qa-assets\monitor-dashboard-k8s-resource-final.png`
- Revised K8s Pod screenshot: `D:\go\ops-admin\design-qa-assets\monitor-dashboard-k8s-final.png`
- Viewport: 1720 × 827 CSS pixels for the primary comparison; 1280 × 800 CSS pixels for responsive verification
- Pixel dimensions and density: source 1720 × 827 px; implementation 1720 × 827 px; device scale factor 1; no density normalization required
- State: light theme, authenticated `node` dashboard, Shanghai test-cluster datasource, one-hour time range, 30-second auto refresh

## Full-view comparison evidence

The issue screenshots and focused implementation capture were opened together in one comparison input. The corrected disk card now contains exactly two node-level rows instead of repeated filesystem/subpath series, displays both full node names, reports two visible series, and keeps values aligned with the bars. The requested ordering is also visible: `内存使用趋势` occupies the former `网络接收速率 Top` position, while the network card moves to the former memory-trend position.

Required fidelity surfaces:

- Fonts and typography: preserved the existing application font stack; headings, KPI values, metadata, and control labels now have distinct weights and compact line heights without clipping or awkward wraps.
- Spacing and layout rhythm: 12 px section rhythm, compact header/filter padding, an aligned 4 × 2 KPI grid, a three-column chart grid, consistent 8–10 px radii, and restrained elevation. Incomplete final chart rows expand to use the available width instead of leaving empty tracks.
- Colors and tokens: white surfaces on `#f3f6fb`, blue active states, and semantic green/amber/red/cyan/purple accents preserve both the reference direction and the current product palette. Visible text and controls retain practical contrast.
- Image quality and asset fidelity: neither screen depends on photographic or illustrative assets. Visible UI symbols use the project's Element Plus vector icon family; no placeholder, custom SVG, CSS-drawn, or raster-substitute assets were introduced.
- Copy and content: dashboard labels remain operational and product-specific. Visible ranking-card series counts now describe the deduplicated rows rather than the raw Prometheus filesystem series count.

## Focused region comparison evidence

The focused 1720 × 827 capture `monitor-dashboard-disk-memory-final-2.png` verifies the exact disk and memory labels, two-row data density, values, complete node names, and the reordered chart positions.

## Findings

- No actionable P0, P1, or P2 findings remain.
- P3: the implementation keeps the application's global navigation and therefore exposes slightly less chart area above the fold than the standalone reference. This is an expected product-shell constraint rather than a local dashboard defect.

## Comparison history

1. Initial implementation capture: the second KPI row contained seven data-backed summary cards and left the eighth grid track empty, creating visible density drift from the reference (P2).
2. Fix: added a derived “监控面板” operational summary card using real active-panel and health counts.
3. User refinement: legacy per-panel span values still produced irregular chart widths and unused horizontal space (P2).
4. Fix: replaced legacy free spans with a six-track packing grid where normal charts span two tracks, producing three equal panels per complete row. A two-panel remainder becomes two half-width cards; a single remainder becomes full width.
5. Post-fix evidence: `monitor-dashboard-three-column-full.png` shows “平均磁盘使用率”, “CPU 使用率 Top”, and “内存使用率 Top” in the same first chart row, all subsequent charts packed without holes, and the final host-information table using the full row.
6. User-reported data fidelity issue: `磁盘使用率 Top` rendered repeated rows for Pod volume-subpath and NFS mounts, truncated the node names, and showed the raw series count (P1). The memory-trend and network-receive cards were also in the wrong requested positions (P2).
7. Fix: prefer root filesystem rows for the disk utilization card, deduplicate ranking cards by `node`/`instance`, prefer the human-readable `node` label, expose complete labels, report the visible row count, and swap the requested panels in the visual ordering layer.
8. Post-fix evidence: `monitor-dashboard-disk-memory-final-2.png` shows exactly `szfc-sh-k8s-node-10.1.6.182` and `szfc-sh-k8s-node-10.1.23.52` for disk and memory rankings, with two visible rows and corrected card positions.

## Interaction and runtime verification

- Switched from `node` to `k8s` and back; the selected dashboard heading and panels updated.
- Verified the first three rendered chart titles in order: “平均磁盘使用率”, “CPU 使用率 Top”, and “内存使用率 Top”.
- Verified the final visualization order places `内存使用趋势` after `系统负载 Top`, and `网络接收速率 Top` after `CPU 使用趋势`.
- Selected the 15-minute quick range; the active button state and data refresh updated.
- Opened the dashboard settings dropdown; its expanded state rendered without layout movement.
- Verified the responsive rules retain two chart columns below 1180 px and one chart column below 760 px; incomplete rows expand rather than leaving a blank column.
- Checked browser warnings/errors after a clean route reload and the interaction pass: no new runtime warnings or errors were emitted. Two earlier transient Vite HMR errors remained in retained browser history but did not recur after reload.
- Production build passed. The only build notice is the existing Vite chunk-size advisory.

## Implementation checklist

- [x] Align information hierarchy with the reference monitoring overview.
- [x] Consolidate data source, time range, auto-refresh, and quick ranges.
- [x] Promote key stat/gauge panels into an eight-card overview grid.
- [x] Standardize desktop chart rows to three equal panels and eliminate incomplete-row whitespace.
- [x] Deduplicate disk and ranking series by node, show complete resource names, and report visible series counts.
- [x] Swap the memory-trend and network-receive panel positions.
- [x] Keep chart data, dashboard switching, refresh, edit, delete, create, and fullscreen behavior intact.
- [x] Verify matched desktop and narrower desktop viewports.
- [x] Run production build and browser runtime checks.

## K8s Pod monitoring iteration

- Viewport and normalization: the implementation was captured at 1720 × 827 CSS pixels with device scale factor 1. The source reference is 2480 × 1082 pixels and represents a wider product shell, so comparison focused on the shared monitoring-card language, density, hierarchy, and readable data rather than literal frame geometry.
- Full-view evidence: `monitor-dashboard-k8s-pod-final.png` shows the K8s resource-distribution row followed by a complete three-column Pod monitoring row. Card headers, semantic status chips, compact metadata, aligned bars, and restrained borders follow the container-management reference direction without leaving empty grid tracks.
- Focused evidence: `monitor-dashboard-k8s-pod-detail-final.png` verifies the Pod CPU and memory trends, receive/send rankings, full `namespace/pod` labels, automatic `m/Core` and `MiB/GiB` units, and a full-width Pod detail table with namespace, Pod, node, Pod IP, and Host IP columns.
- Fonts and typography: existing product fonts and optical hierarchy are preserved. Long Pod names wrap inside ranking labels rather than disappearing, while chart values remain right aligned and scannable.
- Spacing and layout rhythm: Pod panels use the established three-column grid; the detail table expands to the full row. Header, chart-body, and card gaps match the rest of the redesigned dashboard.
- Colors and visual tokens: blue/teal chart accents, green live states, amber empty states, white cards, and cool gray canvas align with the reference and the product's existing semantic palette.
- Image quality and assets: this screen contains no source imagery that requires substitution. Existing Element Plus icons and CSS chart rendering remain consistent and sharp.
- Copy and content: Pod panels use operational names aligned with container monitoring: CPU, memory, cumulative/hourly restarts, trends, network receive/transmit, and detail. Series labels consistently use `namespace/pod`.
- Interactions tested: switched to the K8s dashboard, used the legacy-dashboard “补全 Pod 监控” action, verified it disappeared after success, confirmed the dashboard count changed from 15 to 24, scrolled to Pod charts and Pod details, and observed live query results. No blocking runtime error state appeared during the interaction pass; the earlier clean-reload console check remained clear of new errors.
- Findings: no actionable P0, P1, or P2 visual or functional findings remain. Zero-valued ranking rows now render with zero bar width instead of a misleading minimum-width bar.

### K8s comparison history

1. Existing K8s dashboard contained only 15 panels and omitted Pod resource, restart, network, trend, and detail coverage (P1).
2. Fix: added nine Pod panel definitions based on the container-management monitoring queries and presentation conventions, plus a safe one-click upgrade for existing dashboards.
3. Initial post-fix evidence confirmed 24 panels and live Pod values. A minor data-encoding issue remained where zero-value rankings still showed a short colored bar (P2).
4. Fix: zero values now map to zero visual width while non-zero values retain a small minimum width for visibility.
5. Post-fix evidence: `monitor-dashboard-k8s-pod-final.png` and `monitor-dashboard-k8s-pod-detail-final.png` show accurate bars, readable labels, balanced three-column layout, working trends, network rankings, and the complete Pod table.

## K8s monitoring-detail redesign

- Source visual truth: `C:\Users\Administrator\AppData\Local\Temp\codex-clipboard-20a95bd8-cdf4-43c1-ac40-a6258b215e9b.png`, `codex-clipboard-16855bf9-c9b9-4b71-8b31-538ae27445c4.png`, `codex-clipboard-32247ca1-19cd-45e4-bc79-79619ee55275.png`, and `codex-clipboard-7d7bfad3-a44c-4570-bbe5-7998462a7a9d.png`.
- Implementation evidence: `monitor-dashboard-k8s-resource-final.png` and `monitor-dashboard-k8s-final.png`.
- Viewport: 1280 × 720 CSS pixels at device scale factor 1.25; implementation capture is 1265 × 710 pixels after browser viewport chrome. Source captures range from 2195–2445 × 1234–1245 pixels, so comparison used matched content regions rather than literal full-frame dimensions.
- State: authenticated light-theme K8s dashboard, Shanghai test-cluster datasource, one-hour range, live data, default non-hover card state, plus the no-abnormal-data state.

### Findings and fixes

1. User feedback identified the compact three-column K8s cards as a major mismatch from container monitoring details: plots were too small, Pod names were cramped, and large blank regions had no meaningful state treatment (P1).
2. Fix: K8s now uses a dedicated two-column detail grid while the Node dashboard retains its requested three-column layout. Chart cards were increased to 340 px, chart canvases to 210 px, and legends/labels receive more horizontal space.
3. Fix: Pod CPU, memory, and recent-restart Top panels render as multi-series trends, with Top 10 naming, full time-axis labels, consistent series colors, and a right-aligned current-value badge. Pod network and distribution rankings use ten colored horizontal bars.
4. Initial revised capture exposed a full-width empty abnormal panel that still wasted space (P2).
5. Fix: the resource panels were rebalanced into pairs, PVC was promoted into the overview KPI area, and empty chart cards now show a quiet grid plus an explicit healthy/no-data message instead of an unstructured white void.
6. Post-fix evidence: `monitor-dashboard-k8s-resource-final.png` shows paired distribution/workload panels and a purposeful no-abnormal state; `monitor-dashboard-k8s-final.png` shows the spacious paired Pod CPU and memory charts with value badges, real timestamps, and readable legends.

### Required fidelity surfaces

- Typography: 15 px semibold navy chart titles, compact blue value badges, and muted axis/legend text reproduce the source hierarchy without clipping.
- Spacing and layout: two equal chart columns, 14 px gutters, 58 px headers, 340 px cards, and balanced resource pairs match the monitoring-detail density. The Pod detail table remains full width.
- Colors and tokens: white panels, cool gray canvas, blue header signals/value badges, multi-color series, green healthy state, and amber no-data status align with the supplied screenshots.
- Image and icon fidelity: no photographic or illustrative assets are present. The empty state uses the existing Element Plus icon family; no placeholder or handcrafted SVG asset was introduced.
- Copy and content: chart names now follow the supplied detail wording (`Top 10`, `核`, `MiB`, network flow), and empty-state copy explains whether the state is healthy or requires a datasource/time-range check.
- Responsiveness and accessibility: the two-column K8s grid collapses to one column at mobile width; values are not encoded by color alone, long series labels retain title tooltips, and empty states have visible text.

### Runtime verification

- Switched from Node to K8s and verified 24 panels, live Pod values, updated titles, two-column placement, hover-only edit actions, and explicit empty state.
- Production build passed after the final visual corrections.
- A clean browser tab/reload produced zero console warnings or errors.
- No actionable P0, P1, or P2 findings remain. P3: the app's global navigation reduces chart width compared with the wider source screenshots; this is an expected product-shell constraint.

final result: passed
