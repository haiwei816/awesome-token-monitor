import os
import sys
import json
import time
import threading
import urllib.parse
from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
from collector import UniversalTokenCollector

PORT = 8765
collector = UniversalTokenCollector()

# Background sync worker
def background_sync_worker():
    while True:
        try:
            collector.sync()
        except Exception as e:
            print(f"[Sync Worker Error]: {e}", file=sys.stderr)
        time.sleep(15)

class UniversalMonitorHandler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        web_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "web")
        super().__init__(*args, directory=web_dir, **kwargs)

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path
        query = urllib.parse.parse_qs(parsed.query)

        if path.startswith("/api/"):
            self.handle_api(path, query)
        else:
            if path == "/" or not path:
                self.path = "/index.html"
            super().do_GET()

    def do_POST(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path
        if path == "/api/scan":
            result = collector.sync()
            self.send_json_response(result)
        else:
            self.send_error(404, "Endpoint not found")

    def handle_api(self, path, query):
        try:
            agent = query.get("agent", [None])[0]
            model = query.get("model", [None])[0]
            date = query.get("date", [None])[0]

            if path == "/api/summary":
                data = collector.get_summary(agent_filter=agent, model_filter=model, date_filter=date)
                self.send_json_response(data)
            elif path == "/api/daily":
                limit = int(query.get("limit", [60])[0])
                data = collector.get_daily_records(agent_filter=agent, model_filter=model, limit=limit)
                self.send_json_response(data)
            elif path == "/api/trends":
                granularity = query.get("granularity", ["day"])[0]
                days = int(query.get("days", [30])[0])
                data = collector.get_trends(granularity=granularity, days=days, agent_filter=agent, model_filter=model, date_filter=date)
                self.send_json_response(data)
            elif path == "/api/conversations":
                sort_by = query.get("sort", ["total_tokens"])[0]
                order = query.get("order", ["desc"])[0]
                search = query.get("search", [""])[0]
                limit = int(query.get("limit", [50])[0])
                offset = int(query.get("offset", [0])[0])
                data = collector.get_conversations(
                    agent_filter=agent, model_filter=model, date_filter=date,
                    sort_by=sort_by, order=order, search=search,
                    limit=limit, offset=offset
                )
                self.send_json_response(data)
            elif path.startswith("/api/conversation/"):
                cid = path[len("/api/conversation/"):]
                data = collector.get_conversation_detail(cid)
                if data:
                    self.send_json_response(data)
                else:
                    self.send_json_response({"error": "Conversation not found"}, status=404)
            elif path == "/api/models":
                data = collector.get_models_list()
                self.send_json_response(data)
            elif path == "/api/export":
                fmt = query.get("format", ["json"])[0]
                if fmt == "csv":
                    self.handle_export_csv(agent, model, date)
                else:
                    self.handle_export_json(agent, model, date)
            else:
                self.send_json_response({"error": "API route not found"}, status=404)
        except Exception as e:
            self.send_json_response({"error": str(e)}, status=500)

    def handle_export_csv(self, agent=None, model=None, date=None):
        convs = collector.get_conversations(agent_filter=agent, model_filter=model, date_filter=date, limit=10000)["items"]
        lines = ["Date,Agent,Model,Conversation ID,Title,Project,Steps,Input Tokens,Output Tokens,Cache Tokens,Total Tokens,Last Modified"]
        for c in convs:
            title_escaped = '"' + (c.get('title') or '').replace('"', '""') + '"'
            lines.append(f"{c.get('date_bucket')},{c.get('agent_type')},{c.get('model_display_name')},{c['conversation_id']},{title_escaped},{c['project_id']},{c['step_records_count']},{c['total_input_tokens']},{c['total_output_tokens']},{c['total_cache_tokens']},{c['total_tokens']},{c['last_modified']}")
        csv_data = "\ufeff" + "\n".join(lines)
        content = csv_data.encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "text/csv; charset=utf-8")
        self.send_header("Content-Disposition", 'attachment; filename="universal_ai_token_report.csv"')
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)

    def handle_export_json(self, agent=None, model=None, date=None):
        summary = collector.get_summary(agent_filter=agent, model_filter=model, date_filter=date)
        daily = collector.get_daily_records(agent_filter=agent, model_filter=model)
        trends = collector.get_trends(agent_filter=agent, model_filter=model, date_filter=date)
        convs = collector.get_conversations(agent_filter=agent, model_filter=model, date_filter=date, limit=10000)["items"]
        export_obj = {
            "exported_at": time.strftime("%Y-%m-%d %H:%M:%S"),
            "filter": {"agent": agent or "all", "model": model or "all", "date": date or "all"},
            "summary": summary,
            "daily_records": daily,
            "trends": trends,
            "conversations": convs
        }
        content = json.dumps(export_obj, indent=2, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Disposition", 'attachment; filename="universal_ai_token_data.json"')
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)

    def send_json_response(self, data, status=200):
        content = json.dumps(data, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)

    def log_message(self, format, *args):
        pass

def run_server(port=PORT):
    print(f"[*] Initializing Universal Multi-Agent Token Collector...")
    res = collector.sync()
    print(f"[*] Sync completed in {res.get('duration_seconds', 0)}s. Details: {res.get('counts')}")

    t = threading.Thread(target=background_sync_worker, daemon=True)
    t.start()

    server_address = ("", port)
    try:
        httpd = ThreadingHTTPServer(server_address, UniversalMonitorHandler)
    except OSError as e:
        if "address already in use" in str(e).lower() or e.errno == 10048:
            port += 1
            server_address = ("", port)
            httpd = ThreadingHTTPServer(server_address, UniversalMonitorHandler)
        else:
            raise e

    print(f"================================================================")
    print(f"  Universal AI Agent Token & Model Monitor")
    print(f"  Web Dashboard: http://localhost:{port}")
    print(f"  Supported Agents: Antigravity, Cindy 国际版, Cindy 国内版, Codex, WorkBuddy, Qoder")
    print(f"================================================================")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\n[*] Stopping server...")
        httpd.server_close()

if __name__ == "__main__":
    p = PORT
    if len(sys.argv) > 1:
        try:
            p = int(sys.argv[1])
        except ValueError:
            pass
    run_server(p)
