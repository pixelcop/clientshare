"""Local browser check with mocked API; never sends support mail.

Run against a local Vite server with the Python Playwright package installed:
    python web/src/__tests__/help_browser.py http://127.0.0.1:5199
"""
import json
import os
import sys
from playwright.sync_api import sync_playwright, expect

base = sys.argv[1]
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    for width in [1280, 390]:
        page = browser.new_page(viewport={"width": width, "height": 844})
        state = {"provider": "none", "posts": [], "fail": True, "role": "admin", "booking": False}
        external = []
        page.on("request", lambda req: external.append(req.url) if not req.url.startswith(base) else None)

        def api(route):
            path = route.request.url.split("?")[0]
            data = {}
            status = 200
            if path.endswith("/auth/me"):
                if state["role"] is None:
                    status = 401
                else:
                    data = {"id": "test-user", "role": state["role"], "email": "admin@example.invalid", "name": "Test Admin"}
            elif path.endswith("/auth/passkeys/login/options"):
                data = {"passkey": False, "password": True}
            elif path.endswith("/settings/branding"):
                data = {"site_title": "Test Portal", "docs_url": "/configured-guide/"}
            elif path.endswith("/help") or path.endswith("/help/login"):
                data = {"provider": state["provider"], "guide_url": "/configured-guide/", "support_label": "Contact ClientShare support", "reply_email": "admin@example.invalid", "requester_name": "Test Admin"}
                if state["booking"]:
                    data["booking_url"] = "https://booking.example.invalid/setup"
                if state["provider"] == "operator":
                    data.update(support_label="Contact your portal operator", support_url="https://operator.example.invalid/contact")
            elif path.endswith("/help/messages") or path.endswith("/help/login/messages"):
                state["posts"].append(route.request.post_data_json)
                status = 503 if state["fail"] else 202
                data = {"received": True, "id": "test-receipt"} if status == 202 else {"error": "offline"}
            elif path.endswith("/clients") or path.endswith("/activity"):
                data = []
            route.fulfill(status=status, content_type="application/json", body=json.dumps(data))

        page.route("**/api/**", api)
        # Existing app-wide external font styles are unrelated to Help. Block them
        # in this local check and track only requests made after initial rendering.
        page.route("https://**/*", lambda route: route.fallback() if route.request.url.startswith(base) else route.abort())
        page.goto(base + "/clients")
        page.wait_for_load_state("networkidle")
        external.clear()
        page.get_by_role("button", name="Help", exact=True).click()
        guide_link = page.get_by_role("link", name="User guide", exact=True)
        expect(guide_link).to_be_visible()
        expect(guide_link).to_have_attribute("href", "/configured-guide/")
        guide_link.focus()
        expect(guide_link).to_be_focused()
        dialog = page.get_by_role("dialog")
        expect(dialog.get_by_role("button", name="Getting started: Collect a document")).to_be_visible()
        expect(dialog.get_by_role("button", name="Contact ClientShare support")).to_have_count(0)
        page.keyboard.press("Escape")
        expect(dialog).not_to_be_visible()
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
        # An untouched form returns to the main Help menu when reopened.
        page.keyboard.press("Escape")
        help_button.click()
        expect(dialog.get_by_role("link", name="User guide")).to_be_visible()
        expect(field).to_have_count(0)
        dialog.get_by_role("button", name="Contact ClientShare support").click()
        field.fill("Help me configure this portal")
        # A started draft reopens directly in the form with its content intact.
        page.keyboard.press("Escape")
        help_button.click()
        expect(field).to_have_value("Help me configure this portal")
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
        # The floating launcher opens the same form directly; only internal
        # users may render a booking link, even if capabilities are stale.
        dialog.get_by_role("status").get_by_role("button", name="Close", exact=True).click()
        state["booking"] = True
        page.get_by_role("button", name="Need help?", exact=True).click()
        expect(dialog.get_by_role("link", name="Book a 20-minute setup session")).to_have_attribute("href", "https://booking.example.invalid/setup")
        page.keyboard.press("Escape")
        for role in ["client", "customer", "link"]:
            state["role"] = role
            page.goto(base + "/files")
            page.reload()
            page.wait_for_load_state("networkidle")
            expect(page.get_by_role("button", name="Need help?", exact=True)).to_have_count(0)

        state["role"] = None
        page.goto(base + "/login")
        page.reload()
        page.wait_for_load_state("networkidle")
        widget = page.get_by_role("button", name="Need help?", exact=True)
        expect(widget).to_be_visible()
        widget.focus()
        page.keyboard.press("Enter")
        expect(dialog).to_be_visible()
        expect(dialog.get_by_role("link", name="Book a 20-minute setup session")).to_have_count(0)
        dialog.get_by_label("How can we help?").fill("I cannot sign in")
        dialog.get_by_label("Where should we reply?").fill("visitor@example.invalid")
        bounds = dialog.bounding_box()
        assert bounds["x"] >= 0 and bounds["x"] + bounds["width"] <= width
        assert bounds["y"] >= 0 and bounds["y"] + bounds["height"] <= 844
        assert page.evaluate("document.documentElement.scrollWidth <= window.innerWidth")
        if os.environ.get("HELP_SCREENSHOT_DIR"):
            expect(dialog).to_have_css("opacity", "1")
            page.screenshot(path=f"{os.environ['HELP_SCREENSHOT_DIR']}/help-login-{width}.png", full_page=True, animations="disabled")
        dialog.get_by_role("button", name="Send message").click()
        expect(dialog.get_by_role("status")).to_be_visible()
        assert state["posts"][-1]["email"] == "visitor@example.invalid"
        assert set(state["posts"][-1]) == {"id", "message", "email"}
        page.keyboard.press("Escape")
        expect(widget).to_be_focused()
        # Email-first sign-in moves focus to the password when revealed,
        # including after returning to choose a different email.
        login_email = page.get_by_label("Email", exact=True)
        login_email.fill("visitor@example.invalid")
        login_email.press("Enter")
        expect(page.get_by_label("Password", exact=True)).to_be_focused()
        page.get_by_role("button", name="Use a different email", exact=True).click()
        page.get_by_label("Email", exact=True).press("Enter")
        expect(page.get_by_label("Password", exact=True)).to_be_focused()
        page.close()
    browser.close()
print("Help browser checks passed (desktop and mobile)")
