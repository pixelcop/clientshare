# Demo assets and screenshot provenance

Captured from the public demo opened through **View sample portal** at https://clientshare.app on 2026-10-15. Demo content can be reset independently of the guide.

## Fictional workspaces

- **Willow Creek Holdings:** Getting Started, Documents to Review, Client Uploads, Completed Work.
- **Northstar Design Studio:** Project Briefs, Brand Assets, Final Deliverables.
- **Harbor Community Foundation:** Grant Applications, Reports.

Names, project briefs, checklist rows, and summary figures were invented for the demo. No local user documents were uploaded. Initial staff sample uploads were made with email notification disabled. A later recipient upload used the normal secure-link file-picker workflow with an in-memory fictional text document, `Client response - demo.txt`.

## Public source documents

| Demo filename | Source | Notes |
| --- | --- | --- |
| Sample document.pdf | https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf | W3C public PDF test fixture; uploaded unchanged. |
| Blank IRS Form W-9.pdf | https://www.irs.gov/pub/irs-pdf/fw9.pdf | Blank publicly available US government form; no personal or tax information entered. |
| Public sample - US population.csv | https://raw.githubusercontent.com/plotly/datasets/master/2014_usa_states.csv | Public sample dataset; filename identifies it as sample data. |
| W3C sample image.png | https://www.w3.org/Icons/w3c_home.png | Public W3C image used as a clearly labeled file-preview sample, not as ClientShare branding. |

Other text and CSV files were created in memory specifically for this demonstration. Original public binaries are stored only in the demo; the guide repository contains screenshots rather than a mirror of those documents.

## Capture conventions

Use Firefox with a consistent desktop window. Capture the rendered app, not a mockup. Avoid active link tokens, real email addresses, and account credentials. Images have descriptive alt text in their associated guide pages. A screenshot may show the deployed demo version before the new User Guide navigation item is released.

Included captures: client directory, client invitation dialog (not submitted), secure-link creation, link list, hosted settings, account security, client folders, upload area and files, PDF preview, recipient workspace, completed recipient upload, upload-only link, and activity feed. All 13 captures are cropped to the relevant interface and resized for reading in the guide.

The folder, upload, preview, and link-recipient captures were completed after deployment of the routing fix on branch `fix/folder-role-navigation` (commit `7c2d30a`). Staff folder navigation, the blank W-9 PDF preview, recipient folder navigation, and a real recipient upload were verified against the deployed demo. The resulting upload notification was captured in the staff Activity feed. An additional seven-day Write link was created without sending email to illustrate upload-only access. No access tokens are included in the screenshots or guide.
