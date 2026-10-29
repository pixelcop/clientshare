"""Real browser/API walkthrough against an isolated worktree preview.

uv run --with playwright --with bcrypt python web/src/__tests__/collection_browser.py URL data/clientshare.db
Creates only run-specific fixtures in the supplied copied DB. Outbound email must be disabled.
"""
import datetime
import json
import secrets
import sqlite3
import sys
import uuid
from urllib.parse import urlparse

import bcrypt
from playwright.sync_api import sync_playwright, expect

base, db_path = sys.argv[1:3]
run = uuid.uuid4().hex[:12]
email = f"collect-{run}@example.invalid"
password = secrets.token_urlsafe(24)
db = sqlite3.connect(db_path)
tenant = db.execute("select tenant_id from tenant_domains where domain = ?", (urlparse(base).hostname,)).fetchone()[0]
now = datetime.datetime.now(datetime.timezone.utc).isoformat()
db.execute("insert into users (id,tenant_id,email,password_hash,role,name,created_at,updated_at,passkey_prompt_dismissed) values (?,?,?,?,?,?,?,?,1)",
           (uuid.uuid4().hex[:26], tenant, email, bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode(), "admin", f"Collection check {run}", now, now))
db.commit()
db.close()

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, channel="chrome")
    context = browser.new_context(permissions=["clipboard-read", "clipboard-write"], reduced_motion="reduce")
    page = context.new_page()
    page.goto(base + "/login")
    page.wait_for_load_state("networkidle")
    # Authenticate this run-specific user using the real login endpoint.
    response = context.request.post(base + "/api/auth/login", data={"email": email, "password": password})
    assert response.ok, response.text()
    page.goto(base + "/clients")
    page.wait_for_load_state("networkidle")
    page.get_by_role("button", name="Help", exact=True).click()
    page.get_by_role("button", name="Getting started: Collect a document").click()
    checklist = page.get_by_role("region", name="Collect a document", exact=True)
    expect(checklist).to_be_visible()
    expect(checklist.get_by_role("button", name="Show me where")).to_have_count(0)
    expect(page.locator('#collect-client-search')).to_be_focused()
    page.keyboard.press('Escape')
    page.get_by_role("button", name="New Client", exact=True).click()
    page.get_by_placeholder("Client name").fill(f"Collect browser {run}")
    page.get_by_role("dialog").get_by_role("button", name="Create", exact=True).click()
    expect(checklist.get_by_text("In the client workspace, open Secure Links and select New Link.")).to_be_visible()
    page.reload()
    expect(checklist.get_by_text("In the client workspace, open Secure Links and select New Link.")).to_be_visible()
    progress_data = json.loads(page.evaluate("Object.entries(localStorage).find(([k])=>k.startsWith('clientshare:collect:'))[1]"))
    client_id = progress_data["clientId"]
    client_data = context.request.get(base + f"/api/clients/{client_id}").json()
    upload_url = base + f"/api/folders/{client_data['root_folder_id']}/upload"
    # A file predating this upload request must not count as the response.
    response = context.request.post(upload_url, multipart={"files": {"name": f"preexisting-{run}.txt", "mimeType": "text/plain", "buffer": b"Pre-existing staff file"}})
    assert response.status == 201, response.text()
    page.get_by_role('button', name='Help', exact=True).click()
    page.get_by_role('button', name='Getting started: Collect a document').click()
    expect(page.locator(f'#collect-client-{client_id}')).to_be_focused()
    page.locator(f'#collect-client-{client_id}').click()
    expect(page.locator(f'#collect-links-{client_id}')).to_be_focused()
    page.locator(f'#collect-links-{client_id}').click()
    expect(page.locator('#collect-new-link')).to_be_focused()
    page.screenshot(path='data/collection-real-new-link.png', full_page=True)
    page.get_by_role('button', name='New Link', exact=True).click()
    dialog = page.get_by_role("dialog")
    expect(dialog.get_by_role("checkbox")).not_to_be_checked()
    expect(dialog.get_by_role("combobox").first).to_contain_text("Write")
    dialog.get_by_role("button", name="Create Link", exact=True).click()
    expect(page.get_by_role("button", name="Copy", exact=True)).to_be_visible()
    page.get_by_role("button", name="Copy", exact=True).click()
    expect(checklist.get_by_text("Your link is ready to share.")).to_be_visible()
    # Nor should a staff upload after request creation complete the exchange.
    response = context.request.post(upload_url, multipart={"files": {"name": f"staff-{run}.txt", "mimeType": "text/plain", "buffer": b"Staff file after request"}})
    assert response.status == 201, response.text()
    page.reload()
    expect(checklist.get_by_text("Your link is ready to share.")).to_be_visible()
    link_url = page.evaluate("navigator.clipboard.readText()")
    assert "/links/" in link_url
    progress = page.evaluate("Object.entries(localStorage).find(([k])=>k.startsWith('clientshare:collect:'))[1]")
    assert link_url not in progress and link_url.split("/links/")[1] not in progress
    # A second tab resumes the same request; dismissal/resume propagates.
    tab = context.new_page()
    tab.goto(base + "/clients")
    expect(tab.get_by_text("Your link is ready to share.")).to_be_visible()
    checklist.get_by_role("button", name="Dismiss checklist", exact=True).click()
    expect(tab.get_by_role("region", name="Collect a document", exact=True)).not_to_be_visible()
    tab.get_by_role("button", name="Help", exact=True).click()
    tab.get_by_role("button", name="Getting started: Collect a document").click()
    expect(checklist).to_be_visible()
    tab.close()
    # Verify the mobile checklist is within the viewport.
    page.set_viewport_size({"width": 390, "height": 844})
    bounds = checklist.bounding_box()
    assert bounds["x"] >= 0 and bounds["x"] + bounds["width"] <= 390
    page.get_by_role('button', name='Help', exact=True).click()
    page.get_by_role('button', name='Getting started: Collect a document').click()
    expect(page.locator('#collect-nav-menu')).to_be_focused()
    page.locator('#collect-nav-menu').click()
    expect(page.locator('#collect-nav-clients')).to_have_attribute('aria-describedby', 'collection-coach-description')
    page.screenshot(path='data/collection-real-mobile-menu.png', full_page=True)
    page.locator('#collect-nav-clients').click()
    expect(page.locator(f'#collect-client-{client_id}')).to_be_focused()
    page.locator(f'#collect-client-{client_id}').click()
    page.wait_for_load_state("networkidle")
    # Upload with the actual recipient session in a separate browser context.
    recipient_context = browser.new_context()
    recipient = recipient_context.new_page()
    recipient.goto(link_url)
    recipient.wait_for_load_state("networkidle")
    recipient.locator('input[type="file"]:not([webkitdirectory])').set_input_files({"name": f"document-{run}.txt", "mimeType": "text/plain", "buffer": b"Run-specific collection fixture"})
    recipient.wait_for_load_state("networkidle")
    page.reload()
    expect(checklist.get_by_text("A file was uploaded through this link.")).to_be_visible()
    assert context.request.get(checklist.get_by_role("link", name="Read the guide").get_attribute("href")).ok
    page.screenshot(path=f"data/collection-{run}.png", full_page=True)
    # Restart through the real directory and adopt an existing upload link by copying it.
    page.goto(base + '/clients')
    checklist.get_by_role('button', name='Restart checklist').click()
    expect(page.locator('#collect-client-search')).to_be_focused()
    page.locator('#collect-client-search').fill(f'Collect browser {run}')
    page.locator(f'#collect-client-{client_id}').click()
    expect(page.locator(f'#collect-links-{client_id}')).to_be_focused()
    page.locator(f'#collect-links-{client_id}').click()
    expect(page.locator('#collect-new-link')).to_be_focused()
    page.keyboard.press('Escape')
    # Minimize the instruction panel to browse the wide links table on mobile.
    checklist.get_by_role('button', name='Minimize checklist').click()
    page.get_by_role('button', name='Copy', exact=True).click()
    checklist.get_by_role('button', name='Expand checklist').click()
    expect(checklist.get_by_text('Your link is ready to share.')).to_be_visible()
    progress_data = json.loads(page.evaluate("Object.entries(localStorage).find(([k])=>k.startsWith('clientshare:collect:'))[1]"))
    assert context.request.delete(base + f"/api/links/{progress_data['linkId']}").status == 204
    page.reload()
    expect(checklist.get_by_text("This link no longer accepts uploads. Create a new link or copy an active upload link.")).to_be_visible()
    assert recipient_context.request.get(base + f"/api/folders/{client_data['root_folder_id']}").status == 403
    assert context.request.delete(base + f"/api/clients/{client_id}?confirm=true").ok
    page.goto(base + "/clients")
    expect(checklist.get_by_text('In Clients, select a client’s name to open their workspace. To add one, select New Client.')).to_be_visible()
    recipient_context.close()
    browser.close()
print(f"Collection browser happy path passed: {run}")
