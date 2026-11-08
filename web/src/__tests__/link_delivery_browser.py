"""M2 link creation/result/delivery checks on an isolated email-disabled preview.

uv run --with playwright --with bcrypt python web/src/__tests__/link_delivery_browser.py URL data/clientshare.db
Live checks do not enable outbound email. Queued UI is separately simulated;
durable enqueue/deduplication is covered by the Go integration tests.
"""
import datetime
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
email = f'link-check-{run}@example.invalid'
password = secrets.token_urlsafe(24)
output = Path(db_path).parent
with sqlite3.connect(db_path) as db:
    tenant = db.execute('select tenant_id from tenant_domains where domain = ?', (urlparse(base).hostname,)).fetchone()[0]
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    db.execute('insert into users (id,tenant_id,email,password_hash,role,name,created_at,updated_at,passkey_prompt_dismissed) values (?,?,?,?,?,?,?,?,1)',
               (uuid.uuid4().hex[:26], tenant, email, bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode(), 'admin', f'Link check {run}', now, now))

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, channel='chrome')
    context = browser.new_context(viewport={'width': 1280, 'height': 900}, permissions=['clipboard-read', 'clipboard-write'], reduced_motion='reduce')
    response = context.request.post(base + '/api/auth/login', data={'email': email, 'password': password})
    assert response.ok, response.text()
    response = context.request.post(base + '/api/clients', data={'name': f'Link check {run}'})
    assert response.status == 201, response.text()
    client = response.json()
    endpoint = base + f"/api/clients/{client['id']}/links"
    expired = context.request.post(endpoint, data={'access_type': 'read', 'expiry_days': 1}).json()
    revoked = context.request.post(endpoint, data={'access_type': 'write', 'expiry_days': 1}).json()
    assert context.request.delete(base + f"/api/links/{revoked['id']}").status == 204
    with sqlite3.connect(db_path) as db:
        db.execute('update secure_links set expires_at = ? where id = ?', ('2000-01-01T00:00:00Z', expired['id']))
    page = context.new_page()
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))
    page.goto(base + f"/clients/{client['id']}/links")
    page.wait_for_load_state('networkidle')
    expect(page.get_by_text('No usable links. Create a link to share this client workspace.', exact=True)).to_be_visible()
    page.get_by_role('button', name='Expired & revoked history (2)', exact=True).click()
    expect(page.get_by_role('cell', name='Expired', exact=True)).to_be_visible()
    expect(page.get_by_role('cell').filter(has_text='Revoked').first).to_be_visible()
    expect(page.get_by_role('button', name='Copy', exact=True)).to_have_count(0)
    page.get_by_role('button', name='Usable links (0)', exact=True).click()
    page.get_by_role('button', name='New link', exact=True).click()
    dialog = page.get_by_role('dialog', name=f"Share client workspace: {client['name']}")
    expect(dialog.get_by_text(f"Scope: entire client workspace · {client['name']}", exact=True)).to_be_visible()
    expect(dialog.get_by_role('combobox').first).to_contain_text('View & download')
    dialog.get_by_role('radio', name='Collect uploads', exact=True).check()
    expect(dialog.get_by_role('combobox').first).to_contain_text('Upload only')
    dialog.get_by_role('combobox').first.click()
    page.get_by_role('option', name='View, download & upload', exact=True).click()
    dialog.get_by_role('radio', name='Send documents', exact=True).check()
    dialog.get_by_role('radio', name='Collect uploads', exact=True).check()
    expect(dialog.get_by_role('combobox').first).to_contain_text('Upload only')
    dialog.locator('#collect-link-expiry').fill('7')
    dialog.locator('#collect-link-expiry').press('Tab')
    expect(dialog.get_by_role('region', name='Access summary')).to_contain_text('Anyone with this link')
    page.screenshot(path=str(output / 'link-confirmation-desktop.png'), full_page=True)
    # The server commits the first creation, but the browser loses its response.
    attempts = []
    committed = []
    def creation(route):
        if route.request.method != 'POST':
            route.continue_()
            return
        attempts.append(route.request.headers.get('idempotency-key'))
        if len(attempts) == 1:
            response = route.fetch()
            assert response.status == 201, response.text()
            committed.append(response.json())
            route.abort()
        else:
            route.continue_()
    page.route(endpoint, creation)
    dialog.get_by_role('button', name='Create link', exact=True).click()
    expect(dialog.get_by_role('alert')).to_be_visible()
    dialog.get_by_role('button', name='Create link', exact=True).click()
    expect(dialog.get_by_role('heading', name='Your link is ready', exact=True)).to_be_visible()
    expect(dialog.get_by_role('heading', name='Your link is ready', exact=True)).to_be_focused()
    assert attempts[0] and attempts[0] == attempts[1]
    page.unroute(endpoint, creation)
    created = committed[0]
    assert created['expires_at'] == context.request.get(endpoint).json()[0]['expires_at']
    assert dialog.locator('input[readonly]').input_value() == created['link_url']
    expect(dialog.get_by_text('Not emailed from this dialog. Copying a link does not send an email.', exact=True)).to_be_visible()
    dialog.get_by_role('button', name='Copy link', exact=True).click()
    expect(dialog.get_by_role('button', name='Copied', exact=True)).to_be_visible()
    assert page.evaluate('navigator.clipboard.readText()') == created['link_url']
    # Explicit recipient view never silently uses staff privileges or switches identity.
    with page.expect_popup() as popup:
        dialog.get_by_role('link', name='Open recipient view', exact=True).click()
    recipient_view = popup.value
    expect(recipient_view.get_by_role('heading', name='Open this link as a recipient?')).to_be_visible()
    assert context.request.get(base + '/api/auth/me').json()['role'] == 'admin'
    recipient_view.close()
    # Real disabled-email response; retries keep the existing link and request key.
    dialog.get_by_label('Email this existing link', exact=True).fill('recipient@example.invalid')
    delivery = base + f"/api/links/{created['id']}/email"
    with page.expect_request(delivery) as first:
        dialog.get_by_role('button', name='Email link', exact=True).click()
    expect(dialog.get_by_role('alert')).to_contain_text('Email is unavailable')
    with page.expect_request(delivery) as retry:
        dialog.get_by_role('button', name='Retry email', exact=True).click()
    expect(dialog.get_by_role('alert')).to_contain_text('Email is unavailable')
    assert first.value.headers['idempotency-key'] == retry.value.headers['idempotency-key']
    assert len(context.request.get(endpoint).json()) == 3, 'Retry created another link'
    # Simulated queue acceptance tests presentation only; no real mail is sent.
    page.route(delivery, lambda route: route.fulfill(status=202, json={'status': 'queued', 'link_id': created['id'], 'email_queued': True}))
    dialog.get_by_role('button', name='Retry email', exact=True).click()
    expect(dialog.get_by_text('Email queued. Delivery has not been confirmed.', exact=True)).to_be_visible()
    expect(dialog.get_by_role('button', name='Email link', exact=True)).to_be_disabled()
    page.screenshot(path=str(output / 'link-result-desktop.png'), full_page=True)
    page.set_viewport_size({'width': 390, 'height': 844})
    expect(dialog.get_by_role('button', name='Done', exact=True)).to_be_visible()
    expect(dialog.get_by_role('button', name='Done', exact=True)).to_be_in_viewport()
    bounds = dialog.bounding_box()
    assert bounds['x'] >= 0 and bounds['x'] + bounds['width'] <= 391
    page.screenshot(path=str(output / 'link-result-mobile.png'), full_page=True)
    dialog.get_by_role('button', name='Done', exact=True).click()
    expect(page.get_by_role('button', name='Details & email', exact=True)).to_be_visible()
    page.get_by_role('button', name='Details & email', exact=True).click()
    expect(dialog.get_by_role('heading', name='Your link is ready', exact=True)).to_be_visible()
    expect(dialog.locator('input[readonly]')).to_have_value(created['link_url'])
    expect(dialog.get_by_text('Email queued. Delivery has not been confirmed.', exact=True)).to_be_visible()
    dialog.get_by_role('button', name='Done', exact=True).click()
    assert context.request.delete(base + f"/api/links/{created['id']}").status == 204
    page.reload(wait_until='networkidle')
    expect(page.get_by_text('No usable links. Create a link to share this client workspace.', exact=True)).to_be_visible()
    # New isolated recipient is denied, without reusing staff cookies.
    recipient = browser.new_context()
    assert recipient.request.get(created['link_url']).status == 403
    recipient.close()
    # The embedded new-client settings are not a nested form, and creation
    # navigates to the same persistent result instead of losing the link.
    page.goto(base + '/clients')
    page.wait_for_load_state('networkidle')
    page.get_by_role('button', name='New Client', exact=True).click()
    new_client_dialog = page.get_by_role('dialog')
    new_client_dialog.get_by_placeholder('Client name').fill(f'Created with link {run}')
    new_client_dialog.get_by_role('checkbox', name='Generate secure link', exact=True).check()
    expect(new_client_dialog.locator('form')).to_have_count(1)
    expect(new_client_dialog.get_by_role('combobox').first).to_contain_text('View & download')
    new_client_dialog.get_by_role('button', name='Create', exact=True).click()
    expect(page.get_by_role('heading', name='Your link is ready', exact=True)).to_be_visible()
    assert 'created=' in page.url
    expect(page.get_by_role('dialog')).to_contain_text(f'Created with link {run}')
    assert not errors, errors
    browser.close()
print('PASS: real link creation with lost-response retry (one link), explicit scope/permissions/exact expiry, copy/result persistence, existing-link email retry/unavailable, simulated queued presentation, history/revocation, recipient-session confirmation, desktop/mobile')
