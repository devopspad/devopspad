# Desktop reference design QA

Source: artifacts/reference-desktop-style.png (1920 × 1195, 1×).
Implementation: artifacts/desktop-reference-workbench-iteration-1.png (1920 × 1195 CSS pixels, deviceScaleFactor 1).
Scope: adapt the supplied OfficeDex desktop visual language to Infradex, retaining its database and cluster workflows. Slides, hardware photos, Office ribbon commands, fake OS controls and the reference brand are not part of the requested product.

## Iteration 1
Paired source and rendered screenshot opened together in the comparison tool input. Browser rendering verified through the project's existing Chrome/Playwright workflow; no in-app Browser capability was exposed in the session tools.

- [P2] Left task panel too narrow at the reference viewport (365px versus approximately 426px). Increase it to 420px at large widths while retaining narrower variants for 1480px and 1000px.
- [P2] Assistant text too small relative to the supplied screenshot (13px body, 17px heading). Increase large-screen body to 15px, heading to 19px, and give the prompt and file card more breathing room.
- [P2] Residual green text in table headings, totals and assistant header. Replace it with neutral gray tokens; keep green only for semantic status.
- Behavior regression found and fixed: assistant file card reused the saved-query button name. Rename it to Open current query so the actions remain distinguishable.

## Iteration 2
- The first-pass typography, width and color changes are visible in the paired 1920 × 1195 comparison.
- [P1] At 1480 × 940 and 1000 × 680, the implicit CSS grid row grows to its min-content height, clipping the composer, rail controls and status bar. Evidence: artifacts/desktop-reference-1480-iteration-2.png and artifacts/desktop-reference-1000-iteration-2.png.
- Fix: explicit minmax(0, 1fr) row; min-height: 0 for direct panels and scrolling assistant content; prevent composer shrinking. Add viewport-bound assertions for Send message, Settings and the status bar.
- [P2] Connection inputs still inherit more-specific green borders from the old form styles. Replace at matching specificity and neutralize the preview notice.

## Final comparison

Source and final rendered screenshots were opened together for visual comparison, followed by focused panel inspection. Reference-size rendering uses 1920 × 1195 CSS pixels at deviceScaleFactor 1.

| Surface | Evidence and result |
| --- | --- |
| Layout | [Workbench](artifacts/desktop-reference-workbench.png): narrow rail, 420px left assistant, Explorer and right workspace reproduce the reference's panel hierarchy. Query tabs sit at the top. |
| Typography | [Assistant](artifacts/desktop-reference-assistant.png): system sans-serif, larger task heading and readable prompt/body spacing; compact labels retained in the data workspace. |
| Color and surfaces | Neutral gray task panel, white editor, thin borders and charcoal primary actions; semantic status colors retained. [Connection dialog](artifacts/desktop-reference-dialog.png) now uses matching neutral controls. |
| Icons and assets | Consistent thin outline icons; Infradex identity retained. No decorative bitmap assets are needed for this infrastructure workspace. |
| Density and detail | [Editor and results](artifacts/desktop-reference-editor.png): flat document tabs, compact toolbars, aligned table cells and persistent status bars. Office slide imagery is intentionally replaced by working query/data views. |

Responsive evidence: [1480 × 940](artifacts/desktop-reference-1480.png), [1000 × 680](artifacts/desktop-reference-1000.png), and [Connections](artifacts/desktop-reference-connections.png). The composer, Run query, Settings and status bar remain within the viewport. Assistant content scrolls independently; narrow tables scroll horizontally inside the results region. Long Explorer labels ellipsize.

Validation: 23 Playwright tests passed after the layout redesign; 11 relevant layout/workspace tests passed after the viewport fix; all 3 layout tests passed again after the final narrow-width adjustment. TypeScript unused-code checks and the final Wails desktop build passed. The resulting macOS app remained running during a 3-second startup smoke check. Local Vite preview returned HTTP 200 when checked directly without the environment's proxy.

Remaining minor polish: [P3] the Connections add card and demo CTA retain a small legacy green accent. No blocking visual or interaction findings remain for this reference-style adaptation. This is not a pixel-identical Office editor clone or a formal accessibility audit. Windows/Linux and native WebView visual comparison remain outside this review.

final result: passed
