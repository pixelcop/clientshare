"""Scoped sharing checks. Isolated feature-worktree preview only; no email is sent.

uv run --with playwright --with bcrypt --with pyyaml python web/src/__tests__/sharing_browser.py URL data/clientshare.db
"""
import argparse
import base64
import datetime
import hashlib
import hmac
import secrets
import sqlite3
import time
import uuid
from pathlib import Path
from urllib.parse import urlparse

import bcrypt
import yaml
from playwright.sync_api import expect, sync_playwright

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("url")
parser.add_argument("database")
args = parser.parse_args()
base = args.url.rstrip("/")
repo = Path(__file__).resolve().parents[3]
database = Path(args.database).resolve()
assert database == repo / "data/clientshare.db" and "/work/trees/" in str(repo), "Use an isolated feature worktree database"
config = yaml.safe_load((repo / "config/config.yaml").read_text())
secret = config["auth"]["invite_token_secret"].strip().encode()
run = uuid.uuid4().hex[:10]
password = secrets.token_urlsafe(24)
accounts = {}
with sqlite3.connect(database) as db:
    tenant = db.execute("select tenant_id from tenant_domains where domain = ?", (urlparse(base).hostname,)).fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    for name, role in [("admin", "admin"), ("A", "client"), ("B", "client"), ("folder", "client"), ("writefolder", "client"), ("member", "client")]:
        email = f"sharing-{run}-{name}@example.invalid"
        accounts[name] = {"id": uuid.uuid4().hex[:26], "email": email, "name": f"Sharing {run} {name}"}
        db.execute("insert into users (id,tenant_id,email,password_hash,role,name,created_at,updated_at,passkey_prompt_dismissed) values (?,?,?,?,?,?,?,?,1)",
                   (accounts[name]["id"], tenant, email, bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode(), role, accounts[name]["name"], now, now))

def invitation_token(invitation):
    token = base64.urlsafe_b64encode(hmac.new(secret, ("access-invitation:" + invitation["id"]).encode(), hashlib.sha256).digest()).decode().rstrip("=")
    proof = hmac.new(secret, ("access-redemption:" + token).encode(), hashlib.sha256).hexdigest()
    with sqlite3.connect(database) as db:
        assert db.execute("select token_hash from access_invitations where id = ?", (invitation["id"],)).fetchone()[0] == proof
    return token

def login(context, account):
    for attempt in range(2):
        response = context.request.post(base + "/api/auth/login", data={"email": account["email"], "password": password})
        if response.status != 429 or attempt:
            assert response.ok, f"Login failed: {response.status} {response.text()}"
            return
        # Exercise real auth without disabling the preview's shared login limiter.
        delay = response.headers.get("retry-after") or response.headers.get("x-ratelimit-reset") or "60"
        time.sleep(min(65, max(1, int(delay))) + 1)

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, channel="chrome")
    admin = browser.new_context(viewport={"width": 1280, "height": 900}, reduced_motion="reduce")
    login(admin, accounts["admin"])
    response = admin.request.post(base + "/api/clients", data={"name": f"Private client {run}"})
    assert response.status == 201, response.text()
    client = response.json()
    root = client["root_folder_id"]
    def upload(folder, name):
        response = admin.request.post(base + f"/api/folders/{folder}/upload", multipart={"files": {"name": name, "mimeType": "text/plain", "buffer": b"Scoped sharing browser fixture"}, "notify": "false"})
        assert response.status == 201, response.text()
        return next(item for item in admin.request.get(base + f"/api/folders/{folder}").json()["items"] if item["filename"] == name)
    file = upload(root, "selected.txt")
    private = upload(root, "private-sibling.txt")
    response = admin.request.post(base + f"/api/folders/{root}", data={"name": "Shared folder"})
    assert response.ok, response.text()
    folder = response.json()
    inside = upload(folder["id"], "shared-inside.txt")
    page = admin.new_page()
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.goto(base + f"/folder/{root}")
    page.wait_for_load_state("networkidle")
    assert "Manage client access" in page.get_by_role("button").all_text_contents()

    def open_share(target):
        row = page.get_by_role("row").filter(has=page.get_by_role("button", name=target["filename"], exact=True))
        row.hover()
        row.get_by_role("button", name="Share " + target["filename"], exact=True).click()
        dialog = page.get_by_role("dialog", name="Share: " + target["filename"], exact=True)
        expect(dialog.get_by_label("Recipient email", exact=True)).to_be_visible()
        return dialog

    def invite(dialog, account, reshare=False, upload_access=False, read_access=True):
        dialog.get_by_label("Recipient email", exact=True).fill(account["email"])
        if not read_access:
            dialog.get_by_role("checkbox", name="View & download", exact=True).uncheck()
        if reshare:
            dialog.get_by_role("checkbox", name="Share with others", exact=True).check()
        if upload_access:
            dialog.get_by_role("checkbox", name="Upload", exact=True).check()
        with page.expect_response(lambda response: response.url.endswith("/invitations") and response.request.method == "POST") as created:
            dialog.get_by_role("button", name="Invite", exact=True).click()
        assert created.value.status == 201, created.value.text()
        invitation = created.value.json()
        assert "token" not in invitation and "password" not in invitation
        expect(dialog.get_by_role("status").filter(has_text="Invitation saved.")).to_be_visible()
        return invitation

    def accept(account, invitation):
        context = browser.new_context(viewport={"width": 1280, "height": 900}, reduced_motion="reduce")
        login(context, account)
        recipient = context.new_page()
        recipient.on("pageerror", lambda error: errors.append(str(error)))
        recipient.goto(base + "/accept-access#token=" + invitation_token(invitation))
        recipient.wait_for_load_state("networkidle")
        recipient.get_by_role("button", name="Accept invitation", exact=True).click()
        return context, recipient

    dialog = open_share(file)
    expect(dialog.get_by_text("Sharing: this file only", exact=False)).to_be_visible()
    invitation = invite(dialog, accounts["A"], reshare=True)
    dialog.get_by_role("button", name="Done", exact=True).click()
    named, named_page = accept(accounts["A"], invitation)
    expect(named_page).to_have_url(base + "/files/" + file["id"])
    expect(named_page.get_by_role("heading", name=file["filename"], exact=True)).to_be_visible()
    expect(named_page.get_by_role("button", name="Share", exact=True)).to_be_visible()
    assert named.request.get(base + "/api/folders/" + root).status == 404
    assert named.request.get(base + "/api/files/" + private["id"]).status == 404
    assert named.request.get(base + "/api/clients/" + client["id"] + "/access").status == 404
    named_page.get_by_role("button", name="Share", exact=True).click()
    onward_dialog = named_page.get_by_role("dialog", name="Share: selected.txt", exact=True)
    onward_dialog.get_by_label("Recipient email", exact=True).fill(accounts["B"]["email"])
    with named_page.expect_response(lambda response: response.url.endswith("/invitations") and response.request.method == "POST") as onward:
        onward_dialog.get_by_role("button", name="Invite", exact=True).click()
    assert onward.value.status == 201, onward.value.text()
    onward_context, onward_page = accept(accounts["B"], onward.value.json())
    expect(onward_page).to_have_url(base + "/files/" + file["id"])
    onward_page.goto(base + "/shared-with-me")
    expect(onward_page.get_by_role("link", name="selected.txt View & download", exact=True)).to_be_visible()
    dialog = open_share(file)
    row = dialog.locator(".access-row").filter(has_text=accounts["A"]["name"])
    row.get_by_role("button", name="Remove", exact=True).click()
    confirmation = page.get_by_role("dialog", name="Remove access?", exact=True)
    confirmation.get_by_role("button", name="Remove access", exact=True).click()
    expect(confirmation).not_to_be_visible()
    assert named.request.get(base + "/api/files/" + file["id"]).status == 404
    assert onward_context.request.get(base + "/api/files/" + file["id"] + "/download").ok
    dialog.get_by_role("button", name="Done", exact=True).click()

    dialog = open_share(folder)
    folder_invite = invite(dialog, accounts["folder"], upload_access=True)
    dialog.get_by_role("button", name="Done", exact=True).click()
    folder_context, folder_page = accept(accounts["folder"], folder_invite)
    expect(folder_page).to_have_url(base + "/folder/" + folder["id"])
    expect(folder_page.get_by_role("button", name="Upload", exact=True)).to_be_visible()
    expect(folder_page.get_by_role("button", name="Manage client access", exact=True)).to_have_count(0)
    expect(folder_page.get_by_role("combobox", name="Client workspace", exact=True)).to_have_count(0)
    assert folder_context.request.get(base + "/api/folders/" + root + "/download-zip").status == 404
    assert folder_context.request.get(base + "/api/folders/" + folder["id"] + "/download-zip").ok

    dialog = open_share(folder)
    write_invite = invite(dialog, accounts["writefolder"], upload_access=True, read_access=False)
    dialog.get_by_role("button", name="Done", exact=True).click()
    write_context, write_page = accept(accounts["writefolder"], write_invite)
    expect(write_page).to_have_url(base + "/folder/" + folder["id"])
    expect(write_page.get_by_role("button", name="Upload", exact=True)).to_be_visible()
    expect(write_page.get_by_text(inside["filename"], exact=True)).to_have_count(0)
    assert write_context.request.get(base + "/api/folders/" + folder["id"]).json()["items"] == []
    assert write_context.request.get(base + "/api/files/" + inside["id"] + "/download").status == 404
    with write_page.expect_response(lambda response: response.url.endswith("/upload")) as named_upload:
        write_page.locator('input[type="file"]:not([webkitdirectory])').set_input_files({"name": "named-collection.txt", "mimeType": "text/plain", "buffer": b"Named private collection"})
    assert named_upload.value.status == 201

    # Whole-client invitations remain a separate, explicitly named scope.
    page.get_by_role("button", name="Manage client access", exact=True).click()
    client_dialog = page.get_by_role("dialog", name="Manage client access: " + client["name"], exact=True)
    expect(client_dialog.get_by_text("Sharing: entire client workspace", exact=False)).to_be_visible()
    member_invite = invite(client_dialog, accounts["member"], reshare=True, upload_access=True)
    client_dialog.get_by_role("button", name="Done", exact=True).click()
    member_context, member_page = accept(accounts["member"], member_invite)
    expect(member_page).to_have_url(base + "/folder/" + root)
    expect(member_page.get_by_role("button", name="Manage client access", exact=True)).to_be_visible()
    assert member_context.request.get(base + "/api/files/" + private["id"]).ok

    # Legacy multi-client navigation coexists with direct item-only discovery.
    # Granting a folder in a third client must not add that client to the switcher.
    def create_client(name):
        response = admin.request.post(base + "/api/clients", data={"name": f"{name} {run}"})
        assert response.status == 201, response.text()
        return response.json()

    second_client = create_client("Second workspace")
    item_client = create_client("Item-only workspace")
    second_file = upload(second_client["root_folder_id"], "second-workspace.txt")
    response = admin.request.post(base + "/api/clients/" + second_client["id"] + "/invitations", data={"email": accounts["member"]["email"], "capabilities": {"can_read": True}}, headers={"Idempotency-Key": str(uuid.uuid4())})
    assert response.status == 201, response.text()
    assert member_context.request.post(base + "/api/invitations/accept", data={"token": invitation_token(response.json())}).ok
    response = admin.request.post(base + "/api/folders/" + item_client["root_folder_id"], data={"name": "Item-only shared folder"})
    assert response.ok, response.text()
    item_folder = response.json()
    item_file = upload(item_folder["id"], "item-only.txt")
    response = admin.request.post(base + "/api/files/" + item_folder["id"] + "/grants", data={"user_id": accounts["member"]["id"], "capabilities": {"can_read": True}})
    assert response.status == 201, response.text()
    authorized_ids = {client["id"], second_client["id"]}
    assert {item["id"] for item in member_context.request.get(base + "/api/clients").json()} == authorized_ids
    member_page.reload()
    member_page.wait_for_load_state("networkidle")
    selector = member_page.get_by_role("combobox", name="Client workspace", exact=True)
    expect(selector).to_contain_text(client["name"])

    def select_workspace(target):
        selector.click()
        expect(member_page.get_by_role("option")).to_have_count(2)
        expect(member_page.get_by_role("option", name=item_client["name"], exact=True)).to_have_count(0)
        member_page.get_by_role("option", name=target["name"], exact=True).click()
        expect(member_page).to_have_url(base + "/folder/" + target["root_folder_id"])
        expect(member_page.get_by_role("heading", name=target["name"], exact=True)).to_be_visible()

    select_workspace(second_client)
    expect(member_page.get_by_role("button", name=second_file["filename"], exact=True)).to_be_visible()
    expect(member_page.get_by_role("button", name=private["filename"], exact=True)).to_have_count(0)
    member_page.reload()
    member_page.wait_for_load_state("networkidle")
    expect(selector).to_contain_text(second_client["name"])
    member_page.set_viewport_size({"width": 390, "height": 844})
    expect(selector).to_be_in_viewport()
    select_workspace(client)
    assert member_page.evaluate("document.documentElement.scrollWidth <= innerWidth")
    member_page.screenshot(path=str(database.parent / "multi-client-switcher-mobile.png"), full_page=True)
    member_page.goto(base + "/shared-with-me")
    member_page.get_by_role("link", name="Item-only shared folder View & download", exact=True).click()
    expect(member_page).to_have_url(base + "/folder/" + item_folder["id"])
    expect(member_page.get_by_role("button", name=item_file["filename"], exact=True)).to_be_visible()
    expect(selector).to_contain_text("Choose a client workspace")
    assert member_context.request.get(base + "/api/folders/" + item_client["root_folder_id"]).status == 404
    select_workspace(second_client)
    assert {item["id"] for item in member_context.request.get(base + "/api/clients").json()} == authorized_ids

    # File bearer landings do not infer authority to browse their parent.
    file_link_response = admin.request.post(base + "/api/files/" + file["id"] + "/links", data={"access_type": "read", "expiry_days": 1}, headers={"Idempotency-Key": str(uuid.uuid4())})
    assert file_link_response.status == 201, file_link_response.text()
    file_link = file_link_response.json()
    file_visitor = browser.new_context(viewport={"width": 390, "height": 844})
    file_page = file_visitor.new_page()
    file_page.goto(file_link["link_url"])
    expect(file_page).to_have_url(base + "/files/" + file["id"])
    expect(file_page.get_by_role("heading", name=file["filename"], exact=True)).to_be_visible()
    expect(file_page.get_by_role("button", name="Share", exact=True)).to_have_count(0)
    assert file_visitor.request.get(base + "/api/folders/" + root).status == 404
    assert file_visitor.request.get(base + "/api/files/" + private["id"]).status == 404
    with sqlite3.connect(database) as db:
        db.execute("update secure_links set expires_at='2000-01-01T00:00:00Z' where id=?", (file_link["id"],))
    assert file_visitor.request.get(base + "/api/files/" + file["id"] + "/download").status in (401, 403, 404)

    dialog = open_share(folder)
    page.screenshot(path=str(database.parent / "sharing-dialog-desktop.png"))
    page.evaluate("document.documentElement.classList.add('app-dark')")
    page.screenshot(path=str(database.parent / "sharing-dialog-dark.png"))
    page.evaluate("document.documentElement.classList.remove('app-dark')")
    dialog.get_by_role("combobox", name="Anyone with this link can", exact=True).click()
    page.get_by_role("option", name="Upload only", exact=True).click()
    with page.expect_response(lambda response: response.url.endswith("/links") and response.request.method == "POST") as link_created:
        dialog.get_by_role("button", name="Create link", exact=True).click()
    assert link_created.value.status == 201, link_created.value.text()
    link = link_created.value.json()
    expect(dialog.get_by_role("heading", name="Link ready: Shared folder", exact=True)).to_be_visible()
    assert dialog.get_by_label("Link address", exact=True).input_value() == link["link_url"]
    dialog.get_by_label("Email link to", exact=True).fill("link-recipient@example.invalid")
    for _ in range(2):
        dialog.get_by_role("button", name="Email link", exact=True).click()
        expect(dialog.get_by_role("alert").filter(has_text="Email is unavailable")).to_be_visible()
    with sqlite3.connect(database) as db:
        assert db.execute("select count(*) from secure_links where tenant_id=? and client_id=? and file_id=?", (tenant, client["id"], folder["id"])).fetchone()[0] == 1
    switch = admin.new_page()
    switch.goto(link["link_url"])
    expect(switch.get_by_role("heading", name="Open this link as a recipient?", exact=True)).to_be_visible()
    assert admin.request.get(base + "/api/auth/me").json()["role"] == "admin"
    switch.close()
    page.set_viewport_size({"width": 390, "height": 844})
    expect(dialog.get_by_role("button", name="Done", exact=True)).to_be_in_viewport()
    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
    page.screenshot(path=str(database.parent / "sharing-dialog-mobile.png"), full_page=True)
    collector = browser.new_context(viewport={"width": 390, "height": 844})
    collector_page = collector.new_page()
    collector_page.goto(link["link_url"])
    collector_page.wait_for_load_state("networkidle")
    expect(collector_page.get_by_text(inside["filename"], exact=True)).to_have_count(0)
    expect(collector_page.get_by_role("button", name="Upload", exact=True)).to_be_visible()
    with collector_page.expect_response(lambda response: response.url.endswith("/upload")) as received:
        collector_page.locator('input[type="file"]:not([webkitdirectory])').set_input_files({"name": "collection.txt", "mimeType": "text/plain", "buffer": b"Private collection fixture"})
    assert received.value.status == 201, received.value.text()
    assert collector.request.get(base + "/api/files/" + inside["id"]).status == 404
    assert collector.request.get(base + "/api/folders/" + root).status == 404
    assert admin.request.delete(base + "/api/links/" + link["id"]).status == 204
    assert collector.request.get(base + "/api/folders/" + folder["id"]).status in (401, 403, 404)
    # Membership removal removes that recipient's direct grants and pending intent,
    # while retaining onward access and bearer links unless explicitly revoked.
    with sqlite3.connect(database) as db:
        links_before = db.execute("select count(*) from secure_links where tenant_id=? and client_id=? and revoked_at is null", (tenant, client["id"])).fetchone()[0]
    assert admin.request.delete(base + "/api/clients/" + client["id"] + "/members/" + accounts["member"]["id"], data={"revoke_links": False}).status == 204
    assert member_context.request.get(base + "/api/files/" + private["id"]).status == 404
    assert onward_context.request.get(base + "/api/files/" + file["id"] + "/download").ok
    with sqlite3.connect(database) as db:
        assert db.execute("select count(*) from secure_links where tenant_id=? and client_id=? and revoked_at is null", (tenant, client["id"])).fetchone()[0] == links_before
    assert not errors, errors
    browser.close()
print("PASS: named invitation acceptance, exact file/folder discovery, multi-client workspace switching with item-only navigation, reshare survival, scoped bearer collection, truthful email retries, session-switch confirmation and mobile dialog layout")
