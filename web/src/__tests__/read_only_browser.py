"""Run against an isolated preview with email disabled.

uv run --with playwright --with bcrypt python web/src/__tests__/read_only_browser.py URL data/clientshare.db
"""
import datetime
import secrets
import sqlite3
import sys
import uuid
from urllib.parse import urlparse

import bcrypt
from playwright.sync_api import expect, sync_playwright

base, db_path = sys.argv[1:3]
run = uuid.uuid4().hex[:12]
email = f"readonly-{run}@example.invalid"
password = secrets.token_urlsafe(24)
with sqlite3.connect(db_path) as db:
    tenant = db.execute("select tenant_id from tenant_domains where domain = ?", (urlparse(base).hostname,)).fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    db.execute("insert into users (id,tenant_id,email,password_hash,role,name,created_at,updated_at,passkey_prompt_dismissed) values (?,?,?,?,?,?,?,?,1)",
               (uuid.uuid4().hex[:26], tenant, email, bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode(), "admin", f"Read-only check {run}", now, now))

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, channel="chrome")
    admin = browser.new_context()
    response = admin.request.post(base + "/api/auth/login", data={"email": email, "password": password})
    assert response.ok, response.text()
    response = admin.request.post(base + "/api/clients", data={"name": f"Read-only check {run}"})
    assert response.status == 201, response.text()
    client = response.json()
    root = client["root_folder_id"]
    response = admin.request.post(base + f"/api/folders/{root}/upload", multipart={"files": {"name": "readonly.txt", "mimeType": "text/plain", "buffer": b"Read-only content"}, "notify": "false"})
    assert response.status == 201, response.text()
    file = admin.request.get(base + f"/api/folders/{root}").json()["items"][0]
    response = admin.request.post(base + f"/api/folders/{root}", data={"name": "Nested"})
    assert response.ok, response.text()
    nested = response.json()["id"]

    for access in ["read", "view", "write", "readwrite"]:
        response = admin.request.post(base + f"/api/clients/{client['id']}/links", data={"access_type": access, "expiry_days": 1})
        assert response.status == 201, response.text()
        link = response.json()
        recipient = browser.new_context()
        page = recipient.new_page()
        page.goto(link["link_url"])
        page.wait_for_load_state("networkidle")
        if access in ["read", "view"]:
            expect(page.get_by_text("Read-only", exact=True)).to_be_visible()
            expect(page.get_by_role("button", name="New Folder", exact=True)).to_have_count(0)
            expect(page.get_by_role("button", name="Create account", exact=True)).to_have_count(0)
            expect(page.locator('input[type="file"]')).to_have_count(0)
            expect(page.get_by_role("button", name="Preview", exact=True)).to_be_visible()
            assert recipient.request.get(base + f"/api/files/{file['id']}/download").body() == b"Read-only content"
            assert recipient.request.get(base + f"/api/folders/{root}/download-zip").status == 200
            assert recipient.request.post(base + f"/api/folders/{root}", data={"name": "Forbidden"}).status == 403
            assert recipient.request.post(base + f"/api/folders/{root}/upload", multipart={"files": {"name": "readonly.txt", "mimeType": "text/plain", "buffer": b"Overwrite"}}).status == 403
            assert recipient.request.delete(base + f"/api/files/{file['id']}").status == 403
            assert recipient.request.post(base + "/api/auth/register", data={"token": link["token"], "name": "Viewer", "email": f"viewer-{access}-{run}@example.invalid", "password": password}).status == 403
            page.get_by_text("Nested", exact=True).click()
            page.wait_for_load_state("networkidle")
            expect(page).to_have_url(base + f"/folder/{nested}")
            expect(page.get_by_role("button", name="New Folder", exact=True)).to_have_count(0)
            page.reload(wait_until="networkidle")
            expect(page.get_by_text("Read-only", exact=True)).to_be_visible()
            expect(page.get_by_role("button", name="New Folder", exact=True)).to_have_count(0)
        else:
            expect(page.get_by_role("button", name="New Folder", exact=True)).to_be_visible()
            assert recipient.request.post(base + f"/api/folders/{root}", data={"name": f"Allowed {access}"}).ok
            if access == "write":
                expect(page.get_by_role("button", name="Create account", exact=True)).to_have_count(0)
                assert recipient.request.get(base + f"/api/files/{file['id']}/download").status == 403
            else:
                expect(page.get_by_role("button", name="Create account", exact=True)).to_be_visible()
        recipient.close()
    browser.close()
print("PASS: read/view restrictions, downloads, navigation/reload, registration bypass, and write/readwrite access")
