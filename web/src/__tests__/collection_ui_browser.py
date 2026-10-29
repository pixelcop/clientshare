"""Deterministic empty-state/access/guide browser checks; API fixtures only.
uv run --with playwright python web/src/__tests__/collection_ui_browser.py PREVIEW_URL
"""
import json
import sys
from playwright.sync_api import sync_playwright, expect

base = sys.argv[1]
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, channel="chrome")
    for width, height, guide, provider in [(1280, 844, "/docs/", "none"), (390, 844, "https://guide.example.invalid/manual/", "clientshare"), (320, 568, "/docs/", "none")]:
        context = browser.new_context(viewport={"width": width, "height": height}, reduced_motion="reduce")
        page = context.new_page()
        state = {"role": "admin", "signed_in": False, "clients": []}

        def api(route):
            path = route.request.url.split("?")[0]
            data = {}
            if path.endswith("/auth/me"):
                if not state['signed_in']:
                    route.fulfill(status=401, content_type='application/json', body='{}')
                    return
                data = {"id": "fixture-admin", "tenant_id": "fixture-tenant", "role": state["role"], "name": "Fixture"}
            elif path.endswith('/auth/login'):
                state['signed_in'] = True
            elif path.endswith("/settings/branding"):
                data = {"site_title": "Fixture", "docs_url": guide}
            elif path.endswith("/help"):
                data = {"provider": provider, "guide_url": guide, "support_label": "Contact support"}
            elif path.endswith("/clients"):
                if route.request.method == 'POST':
                    data = {"id": "first-client", "name": route.request.post_data_json['name'], "root_folder_id": "first-folder"}
                    state['clients'].append(data)
                else:
                    data = {"items": state['clients'], "pagination": {"page": 1, "page_size": 20, "total_items": len(state['clients'])}} if "?" in route.request.url else state['clients']
            elif path.endswith('/clients/first-client'):
                data = state['clients'][0]
            elif path.endswith('/collection'):
                data = []
            elif path.endswith("/activity") or path.endswith("/feed"):
                data = {"items": [], "summary": {"unread_count": 0, "read_count": 0}}
            route.fulfill(status=200, content_type="application/json", body=json.dumps(data))

        page.route("**/api/**", api)
        page.goto(base + "/login")
        page.get_by_label('Email', exact=True).fill('first-admin@example.invalid')
        page.get_by_role('button', name='Continue', exact=True).click()
        page.get_by_label('Password', exact=True).fill('fixture-password')
        page.get_by_role('button', name='Sign In', exact=True).click()
        expect(page).to_have_url(base + '/clients')
        page.wait_for_load_state("networkidle")
        expect(page.get_by_role("dialog")).to_have_count(0)
        expect(page.locator('.collection-spotlight')).to_have_count(0)
        expect(page.get_by_role('region', name='Collect a document', exact=True)).to_have_count(0)
        expect(page.get_by_text("Collect your first client document")).to_be_visible()
        page.screenshot(path=f"data/collection-invitation-{width}.png", full_page=True)
        page.get_by_role("button", name="Collect a document", exact=True).click()
        checklist = page.get_by_role("region", name="Collect a document", exact=True)
        expected_guide = (base + guide if guide.startswith("/") else guide) + "clients/manage-clients.html"
        expect(page.locator('#collect-new-client')).to_be_focused()
        expect(page.get_by_role('note', name='Document collection hint')).to_be_visible()
        if width <= 600:
            expect(checklist.get_by_role('button', name='Expand checklist')).to_be_visible()
            assert checklist.bounding_box()['height'] <= 88
        page.screenshot(path=f"data/collection-guidance-{width}.png", full_page=True)
        page.keyboard.press('Escape')
        expect(page.locator('.collection-spotlight')).to_have_count(0)
        expect(checklist.get_by_role("link", name="Read the guide")).to_have_attribute("href", expected_guide)
        # The current step replays guidance without changing progress, for pointer and keyboard.
        saved_progress = page.evaluate("Object.entries(localStorage).find(([key]) => key.startsWith('clientshare:collect:'))[1]")
        step = checklist.get_by_role('tab', name='1 Choose or create a client', exact=True)
        assert step.get_attribute('title') is None
        expect(step).to_have_attribute('aria-description', 'Select to highlight this step in the app.')
        expect(checklist.get_by_role('button', name='Dismiss checklist')).to_have_attribute('title', 'Dismiss checklist (resume from Help)')
        expect(checklist.get_by_role('button', name='Restart checklist')).to_have_attribute('title', 'Restart checklist (resets tour progress only)')
        expect(checklist.get_by_role('button', name='Minimize checklist')).to_have_attribute('title', 'Minimize checklist')
        step.click()
        expect(page.get_by_role('button', name='Close hint')).to_have_attribute('title', 'Close hint (Esc)')
        expect(page.locator('#collect-new-client')).to_be_focused()
        expect(page.get_by_text('YOUR NEXT ACTION', exact=True)).to_have_count(0)
        page.keyboard.press('Escape')
        expect(step).to_be_focused()
        page.keyboard.press('Enter')
        expect(page.locator('#collect-new-client')).to_be_focused()
        page.keyboard.press('Escape')
        page.keyboard.press('Space')
        expect(page.locator('#collect-new-client')).to_be_focused()
        page.keyboard.press('Escape')
        assert page.evaluate("Object.entries(localStorage).find(([key]) => key.startsWith('clientshare:collect:'))[1]") == saved_progress
        if width <= 600:
            assert checklist.bounding_box()['height'] < height * 0.55
            expect(checklist.get_by_role('tab')).to_have_count(1)
            checklist.get_by_role('button', name='All steps', exact=True).click()
            expect(checklist.get_by_role('tab')).to_have_count(4)
            checklist.get_by_role('button', name='Current step', exact=True).click()
        expect(checklist.get_by_role("combobox")).to_have_count(0)
        expect(checklist.get_by_role("button", name="Create a client", exact=True)).to_have_count(0)
        page.get_by_role("button", name="New Client", exact=True).click()
        expect(page.get_by_role("dialog").get_by_placeholder("Client name")).to_be_visible()
        expect(page.locator('#collect-client-name')).to_have_attribute('aria-describedby', 'collection-coach-description')
        page.wait_for_timeout(350)  # Let the dialog's enter transform settle before visual capture.
        coach = page.get_by_role('note', name='Document collection hint')
        bounds = coach.bounding_box()
        assert bounds['x'] >= 0 and bounds['x'] + bounds['width'] <= width
        assert bounds['y'] >= 0 and bounds['y'] + bounds['height'] <= height
        assert page.locator('#collect-client-name').evaluate('(e) => { const r = e.getBoundingClientRect(); return document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2) === e; }')
        page.screenshot(path=f"data/collection-field-{width}.png", full_page=True)
        page.keyboard.press('Escape')
        expect(page.get_by_role('dialog')).to_be_visible()
        page.get_by_role("dialog").get_by_role("button", name="Cancel", exact=True).click()
        expect(checklist.get_by_role("button", name="Show me where")).to_have_count(0)
        checklist.get_by_role("button", name="Minimize checklist").click()
        expect(checklist.locator(".collection-steps")).not_to_be_visible()
        page.reload()
        expect(checklist.locator(".collection-steps")).not_to_be_visible()
        expect(page.locator('.collection-spotlight')).to_have_count(0)
        page.get_by_role("button", name="Help", exact=True).click()
        page.get_by_role("button", name="Getting started: Collect a document").click()
        page.keyboard.press('Escape')
        expect(page.get_by_role('dialog')).to_have_count(0)
        expect(checklist.locator(".collection-steps")).to_be_visible()
        page.screenshot(path=f"data/collection-panel-{width}.png", full_page=True)
        page.evaluate("document.documentElement.classList.add('app-dark')")
        page.screenshot(path=f"data/collection-dark-{width}.png", full_page=True)
        page.evaluate("document.documentElement.classList.remove('app-dark')")
        # A first client is created in the actual form, then the tour points at its directory link.
        page.get_by_role('button', name='New Client', exact=True).click()
        page.get_by_placeholder('Client name').fill('First client')
        expect(page.locator('.collection-spotlight')).to_have_count(0)
        page.get_by_role('dialog').get_by_role('button', name='Create', exact=True).click()
        expect(page.locator('#collect-client-first-client')).to_be_visible()
        expect(page.locator('#collect-client-first-client')).to_have_attribute('aria-describedby', 'collection-coach-description')
        page.keyboard.press('Escape')
        if checklist.get_by_role('button', name='Expand checklist').is_visible():
            checklist.get_by_role('button', name='Expand checklist').click()
        checklist.get_by_role('tab', name='2 Create an upload link', exact=True).click()
        expect(page.locator('#collect-client-first-client')).to_be_focused()
        page.keyboard.press('Escape')
        checklist.get_by_role("button", name="Dismiss checklist", exact=True).click()
        page.reload()
        expect(checklist).not_to_be_visible()
        expect(page.get_by_text("Collect your first client document")).not_to_be_visible()
        for role in ["manager", "client"]:
            state["role"] = role
            page.reload()
            page.wait_for_load_state("networkidle")
            expect(checklist).not_to_be_visible()
            expect(page.get_by_text("Collect your first client document")).not_to_be_visible()
            expect(page.get_by_role("button", name="Getting started: Collect a document")).to_have_count(0)
        context.close()
    browser.close()
print("Collection UI matrix passed: first sign-in, empty directory, first client creation, compact mobile, dismissal/resume, permissions, keyboard, reduced motion, guide configuration")
