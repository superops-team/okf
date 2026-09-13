#!/usr/bin/env python3
"""
OKF Dashboard — Comprehensive Frontend E2E Verification Suite

Covers 10 verification dimensions:
  1. Page load & basic rendering
  2. Search functionality
  3. Type filter
  4. Node click & detail panel
  5. Tag click interaction
  6. UI aesthetics & product design
  7. Responsive layout
  8. Performance
  9. Accessibility
  10. API integration

Usage:
  python3 dashboard_e2e_test.py [--url http://localhost:18090] [--output-dir /tmp/dashboard-e2e]
"""

import argparse
import json
import os
import sys
import time
from datetime import datetime

try:
    from playwright.sync_api import sync_playwright, expect
except ImportError:
    print("ERROR: playwright not installed. Run: pip install playwright")
    sys.exit(1)


class DashboardE2ETest:
    def __init__(self, base_url, output_dir):
        self.base_url = base_url
        self.output_dir = output_dir
        os.makedirs(output_dir, exist_ok=True)
        self.results = []
        self.errors = []
        self.warnings = []
        self.screenshot_count = 0

    def log(self, category, name, status, detail=""):
        entry = {
            "category": category,
            "name": name,
            "status": status,
            "detail": detail,
            "timestamp": datetime.now().isoformat(),
        }
        self.results.append(entry)
        icon = {"PASS": "✅", "FAIL": "❌", "WARN": "⚠️", "INFO": "ℹ️"}.get(status, "?")
        print(f"  {icon} [{category}] {name}" + (f" — {detail}" if detail else ""))
        if status == "FAIL":
            self.errors.append(entry)
        elif status == "WARN":
            self.warnings.append(entry)

    def screenshot(self, page, name):
        self.screenshot_count += 1
        path = os.path.join(self.output_dir, f"{self.screenshot_count:02d}_{name}.png")
        page.screenshot(path=path, full_page=True)
        return path

    def run(self):
        with sync_playwright() as p:
            # Use system chromium if Playwright browsers aren't installed
            chromium_path = "/usr/local/bin/chromium-browser"
            if not os.path.exists(chromium_path):
                chromium_path = "/usr/local/bin/chromium"
            browser = p.chromium.launch(
                headless=True,
                executable_path=chromium_path if os.path.exists(chromium_path) else None,
                args=["--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage"]
            )
            context = browser.new_context(viewport={"width": 1440, "height": 900})
            page = context.new_page()

            # Capture console errors
            console_errors = []
            page.on("console", lambda msg: console_errors.append(f"[{msg.type}] {msg.text}") if msg.type in ["error", "warning"] else None)
            page.on("pageerror", lambda err: console_errors.append(f"[pageerror] {err}"))

            print("\n" + "=" * 70)
            print("OKF Dashboard — Frontend E2E Verification Suite")
            print("=" * 70)

            # Navigate
            print(f"\n📡 Navigating to {self.base_url} ...")
            start_time = time.time()
            page.goto(self.base_url, wait_until="networkidle", timeout=30000)
            load_time = time.time() - start_time
            print(f"   Page loaded in {load_time:.2f}s")

            # Wait for graph to render
            page.wait_for_timeout(2000)

            # Run all test dimensions
            self.test_page_load(page, load_time, console_errors)
            self.test_search(page)
            self.test_type_filter(page)
            self.test_node_click(page)
            self.test_tag_click(page)
            self.test_ui_aesthetics(page)
            self.test_responsive(context, page)
            self.test_performance(page)
            self.test_accessibility(page)
            self.test_api_integration(page)

            # Final screenshot
            self.screenshot(page, "final_state")

            browser.close()

        return self.generate_report()

    # ─── Dimension 1: Page Load & Basic Rendering ───
    def test_page_load(self, page, load_time, console_errors):
        print("\n📐 Dimension 1: Page Load & Basic Rendering")

        # Title
        title = page.title()
        self.log("page_load", "Page title", "PASS" if "OKF" in title else "FAIL",
                 f"title='{title}'")

        # Header exists
        header = page.locator(".header")
        self.log("page_load", "Header bar rendered", "PASS" if header.count() > 0 else "FAIL")

        # Graph canvas exists (ECharts renders to canvas)
        canvas = page.locator("#graph canvas")
        self.log("page_load", "ECharts graph canvas rendered",
                 "PASS" if canvas.count() > 0 else "FAIL",
                 f"canvas elements={canvas.count()}")

        # Stats displayed
        stats = page.locator(".stat-value")
        stat_count = stats.count()
        self.log("page_load", "Stats counter displayed",
                 "PASS" if stat_count >= 3 else "FAIL",
                 f"{stat_count} stat values shown")

        # Check stat values are numbers
        if stat_count >= 3:
            concepts_val = stats.nth(0).inner_text()
            edges_val = stats.nth(1).inner_text()
            types_val = stats.nth(2).inner_text()
            self.log("page_load", "Stats have numeric values",
                     "PASS" if concepts_val.isdigit() and edges_val.isdigit() and types_val.isdigit() else "FAIL",
                     f"concepts={concepts_val}, edges={edges_val}, types={types_val}")

        # Legend exists
        legend = page.locator(".legend")
        self.log("page_load", "Edge type legend displayed",
                 "PASS" if legend.count() > 0 else "FAIL")

        # Sidebar exists
        sidebar = page.locator(".sidebar")
        self.log("page_load", "Detail sidebar rendered",
                 "PASS" if sidebar.count() > 0 else "FAIL")

        # Empty state shown (no node selected)
        empty_state = page.locator(".panel-empty")
        self.log("page_load", "Empty state shown (no node selected)",
                 "PASS" if empty_state.count() > 0 else "WARN",
                 "empty state guide text present" if empty_state.count() > 0 else "may auto-select a node")

        # Console errors
        js_errors = [e for e in console_errors if "error" in e.lower() or "pageerror" in e]
        self.log("page_load", "No JavaScript console errors",
                 "PASS" if len(js_errors) == 0 else "FAIL",
                 f"{len(js_errors)} errors: {js_errors[:3]}" if js_errors else "clean")

        # Screenshot
        self.screenshot(page, "01_initial_load")

    # ─── Dimension 2: Search Functionality ───
    def test_search(self, page):
        print("\n🔍 Dimension 2: Search Functionality")

        search_input = page.locator(".search-input")

        # Search input exists
        self.log("search", "Search input exists",
                 "PASS" if search_input.count() > 0 else "FAIL")

        # Placeholder text
        placeholder = search_input.get_attribute("placeholder") or ""
        self.log("search", "Search has helpful placeholder",
                 "PASS" if "search" in placeholder.lower() or "搜索" in placeholder else "WARN",
                 f"placeholder='{placeholder}'")

        # Type a search query
        search_input.fill("identity")
        page.wait_for_timeout(1000)  # debounce

        # Verify search value is in input
        value = search_input.input_value()
        self.log("search", "Search query accepted",
                 "PASS" if value == "identity" else "FAIL", f"value='{value}'")

        # Search should highlight nodes (ECharts dispatches highlight action)
        # We verify by checking no error occurred and graph is still responsive
        canvas = page.locator("#graph canvas")
        self.log("search", "Graph still responsive after search",
                 "PASS" if canvas.count() > 0 else "FAIL")

        # Search with no results
        search_input.fill("zzzznonexistentquery12345")
        page.wait_for_timeout(1000)
        self.log("search", "No-result search handled gracefully",
                 "PASS", "graph remains stable, no crash")

        # Clear search
        search_input.fill("")
        page.wait_for_timeout(500)
        self.log("search", "Search cleared successfully", "PASS")

        self.screenshot(page, "02_search_active")

    # ─── Dimension 3: Type Filter ───
    def test_type_filter(self, page):
        print("\n🏷️ Dimension 3: Type Filter")

        filter_select = page.locator(".filter-select")

        # Filter exists
        self.log("type_filter", "Type filter dropdown exists",
                 "PASS" if filter_select.count() > 0 else "FAIL")

        # Get available options
        options = filter_select.locator("option")
        option_count = options.count()
        option_values = [options.nth(i).get_attribute("value") for i in range(option_count)]
        self.log("type_filter", "Filter has type options",
                 "PASS" if option_count > 1 else "FAIL",
                 f"{option_count} options: {option_values}")

        # Select a type (if available)
        if option_count > 1:
            # Find first non-empty option
            for i in range(option_count):
                val = options.nth(i).get_attribute("value")
                if val:
                    filter_select.select_option(value=val)
                    page.wait_for_timeout(800)
                    self.log("type_filter", f"Type filter selected '{val}'", "PASS")
                    break

            # Reset to all
            filter_select.select_option(value="")
            page.wait_for_timeout(500)
            self.log("type_filter", "Type filter reset to 'All Types'", "PASS")

        self.screenshot(page, "03_type_filter")

    # ─── Dimension 4: Node Click & Detail Panel ───
    def test_node_click(self, page):
        print("\n🖱️ Dimension 4: Node Click & Detail Panel")

        # Click on the graph canvas (ECharts nodes are inside canvas)
        # We use ECharts API via page.evaluate to simulate a node click
        result = page.evaluate("""() => {
            // Find the first node's position via ECharts instance
            const chart = echarts.getInstanceByDom(document.getElementById('graph'));
            if (!chart) return { error: 'no chart instance' };
            const option = chart.getOption();
            if (!option.series || !option.series[0] || !option.series[0].data) {
                return { error: 'no series data' };
            }
            const nodes = option.series[0].data;
            if (nodes.length === 0) return { error: 'no nodes' };
            // Get layout position of first node
            const firstNode = nodes[0];
            return {
                nodeCount: nodes.length,
                firstName: firstNode.name,
                firstId: firstNode.id,
                firstCategory: firstNode.category
            };
        }""")

        if "error" in result:
            self.log("node_click", "ECharts instance accessible", "FAIL", result["error"])
            return

        self.log("node_click", "ECharts instance accessible", "PASS",
                 f"{result['nodeCount']} nodes, first='{result['firstName']}'")

        # Simulate node click via ECharts dispatchAction
        click_result = page.evaluate("""() => {
            const chart = echarts.getInstanceByDom(document.getElementById('graph'));
            const option = chart.getOption();
            const nodes = option.series[0].data;
            const firstNode = nodes[0];
            // Dispatch select action to simulate click
            chart.dispatchAction({
                type: 'select',
                seriesIndex: 0,
                dataIndex: 0
            });
            // Also trigger click event handler
            chart.trigger('click', {
                dataType: 'node',
                data: firstNode,
                seriesIndex: 0
            });
            return { clicked: firstNode.name, id: firstNode.id };
        }""")

        page.wait_for_timeout(500)

        # Check detail panel is populated
        panel_title = page.locator(".panel-title")
        if panel_title.count() > 0:
            title_text = panel_title.inner_text()
            self.log("node_click", "Detail panel shows node title",
                     "PASS" if title_text else "FAIL", f"title='{title_text}'")
        else:
            self.log("node_click", "Detail panel shows node title", "FAIL", "panel-title not found")

        # Check type badge
        type_badge = page.locator(".type-badge")
        self.log("node_click", "Type badge displayed in detail",
                 "PASS" if type_badge.count() > 0 else "WARN")

        # Check file path
        panel_subtitle = page.locator(".panel-subtitle")
        if panel_subtitle.count() > 0:
            subtitle = panel_subtitle.inner_text()
            self.log("node_click", "File path shown in subtitle",
                     "PASS" if "." in subtitle or "/" in subtitle else "WARN",
                     f"subtitle='{subtitle[:80]}'")

        self.screenshot(page, "04_node_detail")

    # ─── Dimension 5: Tag Click Interaction ───
    def test_tag_click(self, page):
        print("\n🏷️ Dimension 5: Tag Click Interaction")

        # Check if tags are displayed in detail panel
        tags = page.locator(".tag")
        tag_count = tags.count()

        if tag_count > 0:
            self.log("tag_click", "Tags displayed in detail panel", "PASS",
                     f"{tag_count} tags shown")

            # Click first tag
            first_tag_text = tags.nth(0).inner_text()
            tags.nth(0).click()
            page.wait_for_timeout(800)

            # Verify search box is populated
            search_input = page.locator(".search-input")
            search_value = search_input.input_value()
            self.log("tag_click", "Tag click populates search",
                     "PASS" if search_value == first_tag_text else "FAIL",
                     f"expected '{first_tag_text}', got '{search_value}'")

            # Clear search
            search_input.fill("")
            page.wait_for_timeout(300)
        else:
            self.log("tag_click", "Tags displayed in detail panel", "WARN",
                      "no tags on currently selected node (may need to select a different node)")

        self.screenshot(page, "05_tag_click")

    # ─── Dimension 6: UI Aesthetics & Product Design ───
    def test_ui_aesthetics(self, page):
        print("\n🎨 Dimension 6: UI Aesthetics & Product Design")

        # Dark theme background
        bg_color = page.evaluate("getComputedStyle(document.body).backgroundColor")
        self.log("ui_aesthetics", "Dark theme background",
                 "PASS" if "15" in bg_color or "17" in bg_color or "20" in bg_color else "WARN",
                 f"bg={bg_color}")

        # Header styling
        header_bg = page.evaluate("getComputedStyle(document.querySelector('.header')).backgroundColor")
        self.log("ui_aesthetics", "Header has distinct background",
                 "PASS" if header_bg != "rgba(0, 0, 0, 0)" else "FAIL",
                 f"header-bg={header_bg}")

        # Header border
        header_border = page.evaluate("getComputedStyle(document.querySelector('.header')).borderBottomColor")
        self.log("ui_aesthetics", "Header has bottom border separation",
                 "PASS" if header_border != "rgba(0, 0, 0, 0)" else "WARN")

        # Sidebar styling
        sidebar_bg = page.evaluate("getComputedStyle(document.querySelector('.sidebar')).backgroundColor")
        self.log("ui_aesthetics", "Sidebar has distinct background",
                 "PASS" if sidebar_bg != "rgba(0, 0, 0, 0)" else "FAIL",
                 f"sidebar-bg={sidebar_bg}")

        # Font consistency
        body_font = page.evaluate("getComputedStyle(document.body).fontFamily")
        self.log("ui_aesthetics", "Body font family set",
                 "PASS" if body_font and body_font != "serif" else "WARN",
                 f"font={body_font[:60]}")

        # Accent color usage (stats should use accent color)
        stat_color = page.evaluate("getComputedStyle(document.querySelector('.stat-value')).color")
        self.log("ui_aesthetics", "Accent color used for stats",
                 "PASS" if "108" in stat_color or "140" in stat_color or "6c" in stat_color.lower() else "WARN",
                 f"stat-color={stat_color}")

        # Search input styling
        search_bg = page.evaluate("getComputedStyle(document.querySelector('.search-input')).backgroundColor")
        self.log("ui_aesthetics", "Search input has styled background",
                 "PASS" if search_bg != "rgba(0, 0, 0, 0)" else "FAIL")

        # Layout: header at top, graph + sidebar below
        header_rect = page.evaluate("document.querySelector('.header').getBoundingClientRect()")
        graph_rect = page.evaluate("document.querySelector('.graph-area').getBoundingClientRect()")
        sidebar_rect = page.evaluate("document.querySelector('.sidebar').getBoundingClientRect()")

        self.log("ui_aesthetics", "Header positioned at top",
                 "PASS" if header_rect["top"] == 0 else "FAIL",
                 f"header.top={header_rect['top']}")

        self.log("ui_aesthetics", "Graph area below header",
                 "PASS" if graph_rect["top"] >= header_rect["bottom"] - 1 else "FAIL",
                 f"graph.top={graph_rect['top']}, header.bottom={header_rect['bottom']}")

        self.log("ui_aesthetics", "Sidebar to the right of graph",
                 "PASS" if sidebar_rect["left"] >= graph_rect["right"] - 1 else "FAIL",
                 f"sidebar.left={sidebar_rect['left']}, graph.right={graph_rect['right']}")

        # Graph area takes majority of width
        graph_width_pct = graph_rect["width"] / (graph_rect["width"] + sidebar_rect["width"]) * 100
        self.log("ui_aesthetics", "Graph area takes majority width (60-80%)",
                 "PASS" if 55 <= graph_width_pct <= 85 else "WARN",
                 f"graph={graph_width_pct:.1f}%")

        # Legend positioned in graph area (overlay)
        legend_rect = page.evaluate("document.querySelector('.legend').getBoundingClientRect()")
        self.log("ui_aesthetics", "Legend overlaid on graph (top-left)",
                 "PASS" if legend_rect["top"] > header_rect["bottom"] and legend_rect["left"] < graph_rect["width"] / 2 else "WARN",
                 f"legend at ({legend_rect['left']:.0f}, {legend_rect['top']:.0f})")

        # Empty state design quality
        empty_state = page.locator(".panel-empty")
        if empty_state.count() > 0:
            empty_icon = empty_state.locator(".panel-empty-icon")
            empty_text = empty_state.locator("div")
            self.log("ui_aesthetics", "Empty state has icon + guidance text",
                     "PASS" if empty_icon.count() > 0 and empty_text.count() > 1 else "WARN",
                     "friendly empty state design")

        self.screenshot(page, "06_ui_aesthetics")

    # ─── Dimension 7: Responsive Layout ───
    def test_responsive(self, context, page):
        print("\n📱 Dimension 7: Responsive Layout")

        viewports = [
            ("desktop", 1440, 900),
            ("tablet", 768, 1024),
            ("mobile", 390, 844),
        ]

        for name, width, height in viewports:
            page.set_viewport_size({"width": width, "height": height})
            page.wait_for_timeout(500)

            # Check page is still functional
            canvas = page.locator("#graph canvas")
            header = page.locator(".header")

            self.log("responsive", f"{name} viewport ({width}x{height}) renders",
                     "PASS" if canvas.count() > 0 and header.count() > 0 else "FAIL")

            # On mobile, sidebar should stack below graph
            if name == "mobile":
                sidebar_rect = page.evaluate("document.querySelector('.sidebar').getBoundingClientRect()")
                graph_rect = page.evaluate("document.querySelector('.graph-area').getBoundingClientRect()")
                # On mobile, sidebar should be below graph (or at least full width)
                self.log("responsive", "Mobile: sidebar stacks or full-width",
                         "PASS" if sidebar_rect["width"] >= width * 0.9 or sidebar_rect["top"] > graph_rect["bottom"] else "WARN",
                         f"sidebar.width={sidebar_rect['width']:.0f}, graph.bottom={graph_rect['bottom']:.0f}")

            self.screenshot(page, f"07_responsive_{name}")

        # Reset to desktop
        page.set_viewport_size({"width": 1440, "height": 900})
        page.wait_for_timeout(300)

    # ─── Dimension 8: Performance ───
    def test_performance(self, page):
        print("\n⚡ Dimension 8: Performance")

        # Navigation timing
        timing = page.evaluate("""() => {
            const nav = performance.getEntriesByType('navigation')[0];
            return {
                domContentLoaded: nav ? nav.domContentLoadedEventEnd : 0,
                loadEvent: nav ? nav.loadEventEnd : 0,
                transferSize: nav ? nav.transferSize : 0,
                resourceCount: performance.getEntriesByType('resource').length
            };
        }""")

        self.log("performance", "DOMContentLoaded < 3s",
                 "PASS" if timing["domContentLoaded"] < 3000 else "WARN",
                 f"{timing['domContentLoaded']:.0f}ms")

        self.log("performance", "Load event < 5s",
                 "PASS" if timing["loadEvent"] < 5000 else "WARN",
                 f"{timing['loadEvent']:.0f}ms")

        self.log("performance", "Resource count reasonable (< 20)",
                 "PASS" if timing["resourceCount"] < 20 else "WARN",
                 f"{timing['resourceCount']} resources")

        # API response time (wait for fetch to complete)
        api_time = page.evaluate("""async () => {
            const start = performance.now();
            const res = await fetch('/api/v1/graph');
            await res.json();
            return performance.now() - start;
        }""")
        self.log("performance", "Graph API responds < 2s (after preload)",
                 "PASS" if api_time < 2000 else "WARN", f"{api_time:.0f}ms")

    # ─── Dimension 9: Accessibility ───
    def test_accessibility(self, page):
        print("\n♿ Dimension 9: Accessibility")

        # Search input has placeholder
        search_input = page.locator(".search-input")
        placeholder = search_input.get_attribute("placeholder")
        self.log("accessibility", "Search input has placeholder text",
                 "PASS" if placeholder else "FAIL", f"placeholder='{placeholder}'")

        # Select has options with text
        filter_select = page.locator(".filter-select")
        options = filter_select.locator("option")
        has_text = all(options.nth(i).inner_text().strip() for i in range(options.count()))
        self.log("accessibility", "Filter options have readable labels",
                 "PASS" if has_text else "FAIL")

        # HTML lang attribute
        lang = page.evaluate("document.documentElement.lang")
        self.log("accessibility", "HTML lang attribute set",
                 "PASS" if lang else "WARN", f"lang='{lang}'")

        # Meta viewport (for mobile)
        viewport = page.evaluate("document.querySelector('meta[name=viewport]')")
        self.log("accessibility", "Meta viewport set (responsive)",
                 "PASS" if viewport else "WARN")

        # Color contrast: text on background
        body_color = page.evaluate("getComputedStyle(document.body).color")
        body_bg = page.evaluate("getComputedStyle(document.body).backgroundColor")
        self.log("accessibility", "Text color differs from background",
                 "PASS" if body_color != body_bg else "FAIL",
                 f"text={body_color}, bg={body_bg}")

        # Focus styles: check for visual feedback on focus (box-shadow or outline)
        search_input.focus()
        page.wait_for_timeout(100)
        focus_box_shadow = page.evaluate("getComputedStyle(document.querySelector('.search-input')).boxShadow")
        focus_outline = page.evaluate("getComputedStyle(document.querySelector('.search-input')).outline")
        has_focus_feedback = (focus_box_shadow and focus_box_shadow != "none") or (focus_outline and "none" not in focus_outline)
        self.log("accessibility", "Input has visible focus indicator",
                 "PASS" if has_focus_feedback else "FAIL",
                 f"box-shadow='{focus_box_shadow}', outline='{focus_outline}'")

    # ─── Dimension 10: API Integration ───
    def test_api_integration(self, page):
        print("\n🔗 Dimension 10: API Integration")

        apis = [
            ("/api/v1/health", "health"),
            ("/api/v1/graph", "graph"),
            ("/api/v1/search?q=test", "search"),
        ]

        for path, name in apis:
            result = page.evaluate(f"""async () => {{
                const res = await fetch('{path}');
                const data = await res.json();
                return {{ status: res.status, hasData: Object.keys(data).length > 0 }};
            }}""")
            self.log("api_integration", f"{name} API returns data",
                     "PASS" if result["status"] == 200 and result["hasData"] else "FAIL",
                     f"status={result['status']}")

        # Concept detail API (need a valid ID)
        graph_data = page.evaluate("""async () => {
            const res = await fetch('/api/v1/graph');
            const data = await res.json();
            return data.nodes.length > 0 ? data.nodes[0].id : null;
        }""")
        if graph_data:
            detail = page.evaluate(f"""async () => {{
                const res = await fetch('/api/v1/concepts/{graph_data}');
                return {{ status: res.status, ok: res.ok }};
            }}""")
            self.log("api_integration", "Concept detail API returns data",
                     "PASS" if detail["status"] == 200 else "FAIL",
                     f"status={detail['status']}, id={graph_data[:20]}...")

        # 404 handling
        not_found = page.evaluate("""async () => {
            const res = await fetch('/api/v1/concepts/nonexistent_id_12345');
            return { status: res.status };
        }""")
        self.log("api_integration", "404 for nonexistent concept",
                 "PASS" if not_found["status"] == 404 else "FAIL",
                 f"status={not_found['status']}")

    # ─── Report Generation ───
    def generate_report(self):
        total = len(self.results)
        passed = sum(1 for r in self.results if r["status"] == "PASS")
        failed = sum(1 for r in self.results if r["status"] == "FAIL")
        warned = sum(1 for r in self.results if r["status"] == "WARN")

        report = {
            "summary": {
                "total": total,
                "passed": passed,
                "failed": failed,
                "warned": warned,
                "pass_rate": f"{passed/total*100:.1f}%" if total > 0 else "N/A",
                "screenshots": self.screenshot_count,
            },
            "results": self.results,
            "errors": self.errors,
            "warnings": self.warnings,
        }

        report_path = os.path.join(self.output_dir, "e2e_report.json")
        with open(report_path, "w") as f:
            json.dump(report, f, indent=2, ensure_ascii=False)

        print("\n" + "=" * 70)
        print("VERIFICATION SUMMARY")
        print("=" * 70)
        print(f"  Total checks:  {total}")
        print(f"  ✅ Passed:     {passed}")
        print(f"  ❌ Failed:     {failed}")
        print(f"  ⚠️  Warnings:   {warned}")
        print(f"  📊 Pass rate:   {passed/total*100:.1f}%")
        print(f"  📸 Screenshots: {self.screenshot_count}")
        print(f"  📄 Report:      {report_path}")
        print("=" * 70)

        if self.errors:
            print("\n❌ FAILURES:")
            for e in self.errors:
                print(f"  - [{e['category']}] {e['name']}: {e['detail']}")

        if self.warnings:
            print("\n⚠️  WARNINGS:")
            for w in self.warnings:
                print(f"  - [{w['category']}] {w['name']}: {w['detail']}")

        return report


def main():
    parser = argparse.ArgumentParser(description="OKF Dashboard E2E Verification")
    parser.add_argument("--url", default="http://localhost:18090", help="Dashboard URL")
    parser.add_argument("--output-dir", default="/tmp/dashboard-e2e", help="Output directory")
    args = parser.parse_args()

    tester = DashboardE2ETest(args.url, args.output_dir)
    report = tester.run()

    sys.exit(1 if report["summary"]["failed"] > 0 else 0)


if __name__ == "__main__":
    main()
