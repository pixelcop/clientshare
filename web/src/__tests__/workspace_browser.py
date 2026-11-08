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
    page.screenshot(path=str(output / "workspace-empty-desktop.png"), full_page=True)
    page.locator('input[type="file"]:not([webkitdirectory])').set_input_files({"name": "report.txt", "mimeType": "text/plain", "buffer": b"Workspace browser fixture"})
    expect(page.get_by_text("report.txt", exact=True)).to_be_visible()
    expect(page.get_by_role("heading", name="No files yet", exact=True)).to_have_count(0)
    assert abs(upload.bounding_box()["y"] - before_y) < 2
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
    page.screenshot(path=str(output / "workspace-populated-desktop.png"), full_page=True)
    page.set_viewport_size({"width": 390, "height": 844})
    expect(upload).to_be_visible()
    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Mobile overflow"
    page.screenshot(path=str(output / "workspace-populated-mobile.png"), full_page=True)
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
print("PASS: empty/populated/folders-only/error states, stable toolbar, duplicate rename, drag/Escape, read/upload-only, desktop/mobile; no browser errors")
