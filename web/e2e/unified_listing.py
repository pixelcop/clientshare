"""Headless UI regression using synthetic API data; no real account or data writes.

Run with Playwright installed: python web/e2e/unified_listing.py --url <preview-url>
"""
import argparse
import json
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import expect, sync_playwright

parser = argparse.ArgumentParser()
parser.add_argument('--url', required=True)
parser.add_argument('--screenshots', type=Path)
args = parser.parse_args()
base = args.url.rstrip('/')
capabilities = dict(can_read=True, can_upload=True, can_delete=True, can_share=True)
root = dict(id='root', filename='Acme Accounting', client_id='client', type='folder')
children = []
for i in range(65):
    folder = i < 5
    children.append(dict(
        id=f'item{i}', filename=f'Project {i + 1:02}' if folder else f'Document {i - 4:02}.pdf',
        client_id='client', folder_id='root', type='folder' if folder else 'file',
        size=0 if folder else (65-i)*1024,
        uploaded_at=(datetime(2026, 1, 1, tzinfo=timezone.utc)+timedelta(days=i)).isoformat(),
        capabilities=capabilities,
    ))
total = 65
role = 'manager'
requests = []

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport=dict(width=1440, height=1100))
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))

    def api(route):
        parsed = urlparse(route.request.url)
        path = parsed.path
        query = parse_qs(parsed.query)
        if path == '/api/auth/me':
            data = dict(id='test-user', role=role, file_id='root', access_type='read', name='Test User', email='user@example.test')
        elif path == '/api/clients':
            data = [dict(id='client', name=root['filename'], root_folder_id='root')]
        elif path.endswith('/breadcrumbs'):
            folder_id = path.split('/')[3]
            data = [root] if folder_id == 'root' else [root, next(item for item in children if item['id'] == folder_id)]
        elif path.startswith('/api/folders/'):
            requests.append((path, query))
            assert query.get('type') == ['all'], 'The list must use one mixed query, not separate folder queries'
            folder_id = path.split('/')[3]
            rows = children[:total] if folder_id == 'root' else []
            key = query.get('sort_by', ['uploaded_at'])[0]
            descending = query.get('sort_dir', ['desc'])[0] == 'desc'
            folders = [item for item in rows if item['type'] == 'folder']
            files = [item for item in rows if item['type'] == 'file']
            folders.sort(key=lambda item: item['filename'] if key == 'size' else item[key], reverse=False if key == 'size' else descending)
            files.sort(key=lambda item: item[key], reverse=descending)
            rows = folders+files
            size = int(query.get('page_size', ['20'])[0])
            pages = (len(rows)+size-1)//size
            current = max(1, min(int(query.get('page', ['1'])[0]), pages))
            folder = root if folder_id == 'root' else next(item for item in children if item['id'] == folder_id)
            data = dict(
                items=rows[(current-1)*size:current*size],
                pagination=dict(page=current, page_size=size, total_items=len(rows), total_pages=pages, has_more=current < pages),
                context=dict(folder=folder, client_id='client', client_name=root['filename'], root_folder_id='root', capabilities=capabilities, client_capabilities=capabilities, can_manage_client=True, can_manage_queue=False),
            )
        else:
            data = {}
        route.fulfill(status=200, content_type='application/json', body=json.dumps(data))

    page.route('**/api/**', api)

    def settled():
        page.wait_for_load_state('networkidle')
        expect(page.locator('.workspace-content')).to_have_attribute('aria-busy', 'false')

    def names():
        return page.locator('tbody .file-name span').all_text_contents()

    def screenshot(name):
        if args.screenshots:
            args.screenshots.mkdir(parents=True, exist_ok=True)
            page.screenshot(path=str(args.screenshots / name), full_page=True)

    page.goto(base+'/folder/root?page=2&page_size=10&sort_by=filename&sort_dir=asc')
    settled()
    assert len(requests) == 1, requests
    assert len(names()) == 10 and names()[0] == 'Document 06.pdf'
    expect(page.get_by_role('table', name='Files and folders')).to_have_count(1)
    page.get_by_role('button', name='Next Page', exact=True).click()
    settled()
    assert parse_qs(urlparse(page.url).query)['page'] == ['3']
    assert names()[0] == 'Document 16.pdf'
    page.go_back()
    settled()
    assert names()[0] == 'Document 06.pdf'
    page.go_forward()
    settled()
    assert names()[0] == 'Document 16.pdf'

    page.get_by_role('button', name='Name', exact=True).click()
    settled()
    assert 'page' not in parse_qs(urlparse(page.url).query)
    assert names()[:5] == [f'Project {i:02}' for i in range(5, 0, -1)]
    assert page.locator('tbody tr').first.locator('td').nth(1).inner_text() == '—'
    page.get_by_role('button', name='Name', exact=True).click()
    settled()
    assert names()[:5] == [f'Project {i:02}' for i in range(1, 6)]
    screenshot('unified-list-desktop.png')
    for field in ['Added', 'Size']:
        for _ in range(2):
            page.get_by_role('button', name=field, exact=True).click()
            settled()
            assert all(name.startswith('Project') for name in names()[:5])
            if field == 'Size':
                assert names()[:5] == [f'Project {i:02}' for i in range(1, 6)]

    previous = page.url
    page.get_by_role('button', name='Project 01', exact=True).click()
    settled()
    expect(page.get_by_role('heading', name='This folder is empty')).to_be_visible()
    page.go_back()
    settled()
    assert page.url == previous
    page.reload()
    settled()
    assert names()[:5] == [f'Project {i:02}' for i in range(1, 6)]
    page.get_by_role('combobox', name='Rows per page', exact=True).click()
    page.get_by_role('option', name='20', exact=True).click()
    settled()
    assert 'page_size' not in parse_qs(urlparse(page.url).query)
    assert 'page' not in parse_qs(urlparse(page.url).query)
    assert len(names()) == 20
    page.get_by_role('combobox', name='Rows per page', exact=True).click()
    page.get_by_role('option', name='10', exact=True).click()
    settled()
    assert parse_qs(urlparse(page.url).query)['page_size'] == ['10']
    expect(page.get_by_role('listbox')).not_to_be_visible()
    page.set_viewport_size(dict(width=390, height=844))
    screenshot('unified-list-mobile.png')
    assert page.locator('html').evaluate('(el) => el.scrollWidth <= window.innerWidth'), 'Mobile layout overflows'

    page.goto(base+'/folder/root?page=99&page_size=10&sort_by=filename&sort_dir=asc')
    settled()
    assert parse_qs(urlparse(page.url).query)['page'] == ['7']
    assert len(names()) == 5
    total = 18
    page.reload()
    settled()
    assert parse_qs(urlparse(page.url).query)['page'] == ['2']
    assert len(names()) == 8
    total = 0
    page.reload()
    settled()
    assert 'page' not in parse_qs(urlparse(page.url).query)
    expect(page.get_by_role('heading', name='No files yet', exact=True)).to_be_visible()
    # Secure-link visitors also get a canonical folder URL, including when
    # starting at the /folder alias without a folder ID.
    total = 65
    role = 'link'
    page.goto(base+'/folder?page=2&page_size=10&sort_by=filename&sort_dir=asc')
    settled()
    assert urlparse(page.url).path == '/folder/root'
    assert parse_qs(urlparse(page.url).query)['page'] == ['2']
    assert names()[0] == 'Document 06.pdf'
    assert not errors, errors
    print('PASS: unified rows; folder-first sorting; mixed pagination; URL state; Back/Forward; reload; removed/empty pages; desktop/mobile; no page errors')
    browser.close()
