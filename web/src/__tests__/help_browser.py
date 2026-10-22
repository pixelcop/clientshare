"""Local browser check with mocked API; never sends support mail.

Run against a local Vite server with the Python Playwright package installed:
    python web/src/__tests__/help_browser.py http://127.0.0.1:5199
"""
import json
import sys
from playwright.sync_api import sync_playwright, expect

base = sys.argv[1]
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    for width in [1280, 390]:
        page = browser.new_page(viewport={"width": width, "height": 844})
        state = {"provider": "none", "posts": [], "fail": True}
        external = []
        page.on("request", lambda req: external.append(req.url) if not req.url.startswith(base) else None)

        def api(route):
            path = route.request.url.split("?")[0]
            data = {}
            status = 200
            if path.endswith("/auth/me"):
                data = {"id": "test-user", "role": "admin", "email": "admin@example.invalid", "name": "Test Admin"}
            elif path.endswith("/settings/branding"):
                data = {"site_title": "Test Portal", "docs_url": "/configured-guide/"}
            elif path.endswith("/help"):
                data = {"provider": state["provider"], "guide_url": "/configured-guide/", "support_label": "Contact ClientShare support", "reply_email": "admin@example.invalid", "requester_name": "Test Admin"}
                if state["provider"] == "operator":
                    data.update(support_label="Contact your portal operator", support_url="https://operator.example.invalid/contact")
            elif path.endswith("/help/messages"):
                state["posts"].append(route.request.post_data_json)
                status = 503 if state["fail"] else 202
                data = {"received": True, "id": "test-receipt"} if status == 202 else {"error": "offline"}
            elif path.endswith("/clients") or path.endswith("/activity"):
                data = []
            route.fulfill(status=status, content_type="application/json", body=json.dumps(data))

        page.route("**/api/**", api)
        # Existing app-wide external font styles are unrelated to Help. Block them
        # in this local check and track only requests made after initial rendering.
        page.route("https://**/*", lambda route: route.abort())
        page.goto(base + "/clients")
        page.wait_for_load_state("networkidle")
        external.clear()
        guide_link = page.get_by_role("link", name="User guide", exact=True)
        expect(guide_link).to_be_visible()
        expect(guide_link).to_have_attribute("href", "/configured-guide/")
        guide_link.focus()
        expect(guide_link).to_be_focused()
        expect(page.get_by_role("button", name="Help", exact=True)).to_have_count(0)
        dialog = page.get_by_role("dialog")
        expect(dialog).to_have_count(0)
        assert not external, external

        state["provider"] = "operator"
        page.reload()
        page.wait_for_load_state("networkidle")
        help_button = page.get_by_role("button", name="Help", exact=True)
        help_button.focus()
        page.keyboard.press("Enter")
        expect(dialog).to_be_visible()
        expect(dialog.get_by_role("link", name="User guide")).to_have_attribute("href", "/configured-guide/")
        expect(dialog.get_by_role("link", name="Contact your portal operator")).to_have_attribute("href", "https://operator.example.invalid/contact")
        page.keyboard.press("Escape")
        expect(dialog).not_to_be_visible()
        expect(help_button).to_be_focused()
        state["provider"] = "clientshare"
        page.reload()
        page.wait_for_load_state("networkidle")
        external.clear()
        help_button.click()
        dialog.get_by_role("button", name="Contact ClientShare support").click()
        expect(dialog.get_by_text("Tell us what you need a hand with. Your message goes straight to the ClientShare support team.")).to_be_visible()
        expect(dialog.get_by_text("admin@example.invalid", exact=False)).to_have_count(0)
        expect(dialog.get_by_text("Test Admin", exact=False)).to_have_count(0)
        expect(dialog.get_by_text("Book a 20-minute setup session")).to_have_count(0)
        field = dialog.get_by_label("How can we help?")
        field.fill("Help me configure this portal")
        dialog.get_by_role("button", name="Send message").click()
        expect(dialog.get_by_role("alert")).to_be_visible()
        expect(field).to_have_value("Help me configure this portal")
        state["fail"] = False
        dialog.get_by_role("button", name="Send message").click()
        expect(dialog.get_by_text("Thanks for reaching out! We’ve received your message and will reply by email.")).to_be_visible()
        assert len(state["posts"]) == 2
        assert state["posts"][0] == state["posts"][1]
        assert set(state["posts"][0]) == {"id", "message", "screen"}
        assert state["posts"][0]["screen"] == "clients"
        bounds = dialog.bounding_box()
        assert bounds["x"] >= 0 and bounds["x"] + bounds["width"] <= width
        assert not external, external
        page.close()
    browser.close()
print("Help browser checks passed (desktop and mobile)")
