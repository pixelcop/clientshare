"""Workspace interaction checks on an isolated, email-disabled worktree preview.

uv run --with playwright --with bcrypt python web/src/__tests__/workspace_browser.py URL data/clientshare.db
Creates run-specific fixtures only. Never use a production or main-checkout DB.
"""
import datetime
import json
import secrets
import sqlite3
import sys
import uuid
from pathlib import Path
from urllib.parse import urlparse

import bcrypt
from playwright.sync_api import expect, sync_playwright

base, db_path = sys.argv[1:3]
run = uuid.uuid4().hex[:12]
email = f"workspace-{run}@example.invalid"
password = secrets.token_urlsafe(24)
output = Path(db_path).parent
with sqlite3.connect(db_path) as db:
    tenant = db.execute("select tenant_id from tenant_domains where domain = ?", (urlparse(base).hostname,)).fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    db.execute("insert into users (id,tenant_id,email,password_hash,role,name,created_at,updated_at,passkey_prompt_dismissed) values (?,?,?,?,?,?,?,?,1)",
               (uuid.uuid4().hex[:26], tenant, email, bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode(), "admin", f"Workspace check {run}", now, now))

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, channel="chrome")
    admin = browser.new_context(viewport={"width": 1280, "height": 900}, reduced_motion="reduce")
    response = admin.request.post(base + "/api/auth/login", data={"email": email, "password": password})
    assert response.ok, response.text()
    response = admin.request.post(base + "/api/clients", data={"name": f"Workspace check {run}"})
    assert response.status == 201, response.text()
    client = response.json()
    root = client["root_folder_id"]
    page = admin.new_page()
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.goto(base + f"/folder/{root}")
    page.wait_for_load_state("networkidle")
    expect(page.get_by_role("heading", name="No files yet", exact=True)).to_be_visible()
    upload = page.get_by_role("button", name="Upload", exact=True)
    before_y = upload.bounding_box()["y"]
    expect(page.get_by_role("button", name="New folder", exact=True)).to_be_visible()
    page.get_by_role('button', name='Share', exact=True).click()
    access_dialog = page.get_by_role('dialog', name=f"Share workspace: {client['name']}")
    expect(access_dialog.get_by_text('Entire client workspace', exact=False)).to_be_visible()
    expect(access_dialog.get_by_label('Add people', exact=True)).to_be_visible()
    expect(access_dialog.get_by_role('link', name='Manage people')).to_have_count(0)
    access_dialog.get_by_role('tab', name='Links', exact=True).click()
    expect(access_dialog.get_by_role('link', name='Manage links')).to_be_visible()
    page.keyboard.press('Escape')
    page.get_by_role('button', name='Pending email', exact=True).click()
    expect(page.get_by_role('dialog', name='Pending email for this client')).to_be_visible()
    page.keyboard.press('Escape')
    page.screenshot(path=str(output / "workspace-empty-desktop.png"), full_page=True)
    page.locator('input[type="file"]:not([webkitdirectory])').set_input_files({"name": "report.txt", "mimeType": "text/plain", "buffer": b"Workspace browser fixture"})
    expect(page.get_by_text("report.txt", exact=True)).to_be_visible()
    expect(page.get_by_role("heading", name="No files yet", exact=True)).to_have_count(0)
    assert abs(upload.bounding_box()["y"] - before_y) < 2
    # Returning to the window must not recheck auth or clear the workspace,
    # even with the global collection dock mounted.
    auth_requests = []
    page.on('request', lambda request: auth_requests.append(request.url) if request.url.endswith('/api/auth/me') else None)
    page.evaluate("document.dispatchEvent(new Event('visibilitychange')); window.dispatchEvent(new Event('focus')); window.dispatchEvent(new PageTransitionEvent('pageshow', {persisted: true}))")
    page.wait_for_timeout(250)
    assert len(auth_requests) == 0, f'Expected no focus auth checks, got {len(auth_requests)}'
    expect(page.get_by_role('heading', name=client['name'], exact=True)).to_be_visible()
    expect(page.get_by_text('report.txt', exact=True)).to_be_visible()
    expect(upload).to_be_enabled()
    assert page.evaluate("document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get('fileManager').currentFolder") == root
    page.get_by_text("Upload options", exact=False).click()
    page.get_by_role("checkbox", name="Notify by email after upload").uncheck()
    # A second selection retains duplicate resolution rather than overwriting.
    expect(upload).to_be_enabled()
    page.locator('input[type="file"]:not([webkitdirectory])').set_input_files({"name": "report.txt", "mimeType": "text/plain", "buffer": b"Renamed fixture"})
    dialog = page.get_by_role("dialog", name="Rename duplicate files")
    expect(dialog).to_be_visible()
    dialog.get_by_role("textbox").fill("renamed.txt")
    dialog.get_by_role("button", name="Upload", exact=True).click()
    expect(page.get_by_text("renamed.txt", exact=True)).to_be_visible()
    expect(dialog).not_to_be_visible()
    # Internal/text drags do not trigger file upload; a real file drag does.
    page.locator('.file-workspace').evaluate("el => { const d = new DataTransfer(); d.setData('text/plain', 'text'); el.dispatchEvent(new DragEvent('dragover', {bubbles:true, dataTransfer:d})); }")
    expect(page.locator('.upload-highlight')).to_have_count(0)
    expect(upload).to_be_enabled()
    page.locator('.file-workspace').evaluate("el => { const d = new DataTransfer(); d.items.add(new File(['x'],'drag.txt')); el.dispatchEvent(new DragEvent('dragover', {bubbles:true, dataTransfer:d})); }")
    expect(page.locator('.upload-highlight')).to_be_visible()
    page.keyboard.press('Escape')
    expect(page.locator('.upload-highlight')).to_have_count(0)
    # Quick actions work from hover and keyboard; More/Escape returns focus.
    row = page.get_by_role('row').filter(has=page.get_by_role('button', name='report.txt', exact=True))
    row.hover()
    assert row.locator('xpath=ancestor::table/parent::*').evaluate('el => el.scrollWidth <= el.clientWidth'), 'Reserved actions must not overflow the list or scroll its names out of view'
    preview = row.get_by_role('button', name='Preview report.txt', exact=True)
    expect(preview).to_be_visible()
    more = row.get_by_role('button', name='More actions for report.txt', exact=True)
    more.click()
    expect(page.get_by_role('menuitem', name='Preview', exact=True)).to_be_visible()
    expect(page.get_by_role('menuitem', name='Preview', exact=True)).to_be_in_viewport()
    print('Row menu bounds:', page.get_by_role('menuitem', name='Preview', exact=True).bounding_box())
    expect(preview).to_be_visible()
    page.keyboard.press('Escape')
    expect(more).to_be_focused()
    more.click()
    page.screenshot(path=str(output / 'workspace-row-menu-desktop.png'))
    page.keyboard.press('Escape')
    row.get_by_role('button', name='report.txt', exact=True).focus()
    page.keyboard.press('Tab')
    expect(preview).to_be_focused()
    page.keyboard.press('Enter')
    expect(page.get_by_role('dialog', name='Preview report.txt', exact=True)).to_have_count(1)
    page.keyboard.press('Escape')
    # Download does not accidentally activate the filename/preview action.
    with page.expect_download() as download:
        row.get_by_role('button', name='Download report.txt', exact=True).click()
    assert download.value.suggested_filename == 'report.txt'
    expect(page.get_by_role('dialog')).to_have_count(0)
    # Folder picker keeps nested structure and ignores common junk files.
    selected = output / f'Incoming-{run}'
    (selected / '2026').mkdir(parents=True)
    (selected / '2026' / 'notes.txt').write_text('Directory picker fixture')
    (selected / '.DS_Store').write_text('ignored')
    expect(upload).to_be_enabled()
    with page.expect_response(lambda response: response.url.endswith('/upload')) as uploaded:
        page.locator('input[webkitdirectory]').set_input_files(str(selected.resolve()))
    assert uploaded.value.status == 201
    expect(page.get_by_role('button', name=selected.name, exact=True)).to_be_visible()
    incoming = next(item for item in admin.request.get(base + f'/api/folders/{root}?type=folder').json()['items'] if item['filename'] == selected.name)
    assert admin.request.get(base + f"/api/folders/{incoming['id']}").json()['items'] == []
    year = admin.request.get(base + f"/api/folders/{incoming['id']}?type=folder").json()['items'][0]
    assert admin.request.get(base + f"/api/folders/{year['id']}").json()['items'][0]['filename'] == 'notes.txt'
    # Coarse-pointer users do not depend on hover to discover the common action.
    touch = browser.new_context(storage_state=admin.storage_state(), viewport={"width": 390, "height": 844}, has_touch=True, is_mobile=True)
    touch_page = touch.new_page()
    touch_page.goto(base + f'/folder/{root}')
    touch_page.wait_for_load_state('networkidle')
    expect(touch_page.get_by_role('button', name='Preview report.txt', exact=True)).to_be_visible()
    touch_page.get_by_role('button', name='More actions for report.txt', exact=True).click()
    expect(touch_page.get_by_role('menuitem', name='Download', exact=True)).to_be_visible()
    touch_page.keyboard.press('Escape')
    expect(touch_page.get_by_role('menu')).to_have_count(0)
    touch_page.screenshot(path=str(output / 'workspace-touch-mobile.png'), full_page=True)
    touch.close()
    expect(upload).to_be_enabled()
    page.screenshot(path=str(output / "workspace-populated-desktop.png"), full_page=True)
    page.set_viewport_size({"width": 390, "height": 844})
    expect(upload).to_be_visible()
    page.screenshot(path=str(output / "workspace-populated-mobile.png"), full_page=True)
    page.wait_for_function("document.documentElement.scrollWidth <= innerWidth")
    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), page.evaluate("[...document.querySelectorAll('body *')].filter(el=>el.getBoundingClientRect().right>innerWidth+1).map(el=>({tag:el.tagName,cls:el.className,right:el.getBoundingClientRect().right})).slice(0,12)")
    # A failed folder query must never appear as an empty folder.
    page.route(f"**/api/folders/{root}?*", lambda route: route.fulfill(status=503, json={"error": "Fixture folder unavailable"}) if "type=folder" in route.request.url else route.continue_())
    page.reload(wait_until="networkidle")
    expect(page.get_by_role("alert").filter(has_text="Fixture folder unavailable")).to_be_visible()
    expect(page.get_by_role("heading", name="No files yet", exact=True)).to_have_count(0)
    page.unroute(f"**/api/folders/{root}?*")
    page.get_by_role("button", name="Try again").click()
    expect(page.get_by_text("report.txt", exact=True)).to_be_visible()
    # Folders alone mean populated; empty-folder copy differs from root copy.
    folder_response = admin.request.post(base + f"/api/folders/{root}", data={"name": "Documents"})
    assert folder_response.ok, folder_response.text()
    nested = folder_response.json()["id"]
    for file in admin.request.get(base + f"/api/folders/{root}").json()["items"]:
        assert admin.request.delete(base + f"/api/files/{file['id']}").status == 204
    page.reload(wait_until="networkidle")
    expect(page.get_by_text("Documents", exact=True)).to_be_visible()
    expect(page.get_by_role("heading", name="No files yet", exact=True)).to_have_count(0)
    page.get_by_text("Documents", exact=True).click()
    expect(page.get_by_role("heading", name="This folder is empty", exact=True)).to_be_visible()
    # Read-only and upload-only contexts use distinct layouts and instructions.
    for access in ["read", "write"]:
        response = admin.request.post(base + f"/api/clients/{client['id']}/links", data={"access_type": access, "expiry_days": 1})
        assert response.status == 201, response.text()
        recipient = browser.new_context(viewport={"width": 390, "height": 844})
        recipient_page = recipient.new_page()
        recipient_page.goto(response.json()["link_url"])
        recipient_page.wait_for_load_state("networkidle")
        if access == "read":
            recipient_page.get_by_text("Documents", exact=True).click()
            expect(recipient_page.get_by_role("heading", name="No files shared here yet", exact=True)).to_be_visible()
            expect(recipient_page.get_by_role("button", name="Upload", exact=True)).to_have_count(0)
            expect(recipient_page.locator('input[type="file"]')).to_have_count(0)
        else:
            expect(recipient_page.get_by_role("heading", name=f"Upload documents to {client['name']}")).to_be_visible()
            expect(recipient_page.get_by_text("Documents", exact=True)).to_have_count(0)
        recipient_page.screenshot(path=str(output / f"workspace-{access}-mobile.png"), full_page=True)
        recipient.close()
    assert not errors, errors
    (output / "workspace-browser-fixture.json").write_text(json.dumps({"client_id": client["id"], "root_folder_id": root}))
    browser.close()
print("PASS: empty/populated/folders-only/error states, stable toolbar, file/folder pickers, duplicate rename, drag/Escape, keyboard/More focus return, download isolation, touch actions, contextual access, read/upload-only, desktop/mobile; no browser errors")
