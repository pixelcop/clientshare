"""Check actual replay payloads on an isolated, email-disabled dev preview.

uv run --with playwright --with bcrypt python web/src/__tests__/replay_privacy_browser.py URL data/clientshare.db
Creates synthetic fixtures in the preview only. Never use a production/main DB.
Replay envelopes are inspected in memory, never uploaded to Sentry.
"""
import argparse
import datetime
import secrets
import sqlite3
import uuid
from pathlib import Path
from urllib.parse import urlparse

import bcrypt
from playwright.sync_api import expect, sync_playwright

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("url")
parser.add_argument("database")
args = parser.parse_args()
base = args.url.rstrip("/")
run = uuid.uuid4().hex[:12]
email = f"replay-{run}@example.invalid"
password = secrets.token_urlsafe(24)
operator = f"Private Operator {run}"
client_name = f"Private Client {run}"
filename = f"private-report-{run}.txt"
content = f"Confidential document content {run}"
output = Path(args.database).parent

with sqlite3.connect(args.database) as db:
    tenant = db.execute("select tenant_id from tenant_domains where domain = ?", (urlparse(base).hostname,)).fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    db.execute("insert into users (id,tenant_id,email,password_hash,role,name,created_at,updated_at,passkey_prompt_dismissed) values (?,?,?,?,?,?,?,?,1)",
               (uuid.uuid4().hex[:26], tenant, email, bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode(), "admin", operator, now, now))


def start_capture(page):
    page.evaluate("""async () => {
      const source = await fetch('/src/services/runtimeReplay.ts').then(r => r.text());
      const modulePath = source.match(/from ["']([^"']*@sentry_vue[^"']*)["']/)[1];
      const Sentry = await import(modulePath);
      const { replayPrivacyOptions } = await import('/src/services/replayPrivacy.ts');
      window.__privacyRecordings = [];
      Sentry.init({
        dsn: 'https://public@example.invalid/1',
        defaultIntegrations: false,
        replaysSessionSampleRate: 0,
        replaysOnErrorSampleRate: 0,
        integrations: [Sentry.replayIntegration({ ...replayPrivacyOptions, useCompression: false, minReplayDuration: 0 })],
        transport: () => ({
          send: async envelope => {
            for (const [header, payload] of envelope[1]) {
              if (header.type === 'replay_recording') {
                window.__privacyRecordings.push(typeof payload === 'string' ? payload : new TextDecoder().decode(payload));
              }
            }
            return { statusCode: 200 };
          },
          flush: async () => true,
        }),
      });
      Sentry.getReplay().start();
      window.__privacyFlush = async () => {
        await Sentry.getReplay().flush();
        return window.__privacyRecordings.join('');
      };
    }""")


def recording(page, readable):
    # Let rrweb's MutationObserver process UI changes before flushing.
    page.wait_for_timeout(100)
    payload = page.evaluate("window.__privacyFlush()")
    assert readable in payload, f"Expected readable UI: {readable}"
    for private in [email, operator, client_name, filename, content, f"recipient-{run}@example.invalid"]:
        assert private not in payload, f"Sensitive fixture escaped masking: {private}"


with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, channel="chrome")
    context = browser.new_context(viewport={"width": 1280, "height": 900}, reduced_motion="reduce")
    response = context.request.post(base + "/api/auth/login", data={"email": email, "password": password})
    assert response.ok, response.text()
    response = context.request.post(base + "/api/clients", data={"name": client_name})
    assert response.status == 201, response.text()
    client = response.json()
    page = context.new_page()
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.goto(base + f"/folder/{client['root_folder_id']}")
    page.wait_for_load_state("networkidle")
    expect(page.get_by_role("heading", name=client_name, exact=True)).to_be_visible()
    page.locator('input[type="file"]:not([webkitdirectory])').set_input_files({"name": filename, "mimeType": "text/plain", "buffer": content.encode()})
    row = page.get_by_role("row").filter(has=page.get_by_role("button", name=filename, exact=True))
    expect(row).to_be_visible()
    start_capture(page)
    recording(page, "Upload")
    page.get_by_role("button", name="Share workspace", exact=True).click()
    dialog = page.get_by_role("dialog", name=f"Share workspace: {client_name}")
    expect(dialog).to_be_visible()
    dialog.locator('input[type="email"]').fill(f"recipient-{run}@example.invalid")
    dialog.get_by_role("button", name="Add", exact=True).click()
    expect(dialog.locator(".recipient-identity")).to_be_visible()
    recording(page, "Send invitation")
    page.screenshot(path=str(output / "replay-privacy-sharing-desktop.png"), full_page=True)
    # Use the store's authorized listing instead of guessing identifiers.
    file_id = page.evaluate("filename => document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get('fileManager').items.find(item => item.filename === filename).id", filename)
    page.goto(base + f"/files/{file_id}")
    page.wait_for_load_state("networkidle")
    expect(page.locator(".text-preview code")).to_have_text(content)
    start_capture(page)
    recording(page, "Download")
    page.screenshot(path=str(output / "replay-privacy-file-desktop.png"), full_page=True)
    page.set_viewport_size({"width": 390, "height": 844})
    expect(page.get_by_role("button", name="Share", exact=True)).to_be_visible()
    page.screenshot(path=str(output / "replay-privacy-file-mobile.png"), full_page=True)
    recording(page, "Download")
    assert not errors, errors
    browser.close()
print("Replay privacy browser checks passed; no recordings uploaded.")
