"""Health check endpoint for Python workers."""

import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class HealthHandler(BaseHTTPRequestHandler):
    """Minimal HTTP handler for health checks."""

    def do_GET(self) -> None:  # noqa: N802 — required by BaseHTTPRequestHandler
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok"}).encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, fmt: str, *args: object) -> None:
        pass  # Suppress default stderr logging


def serve(port: int = 8081) -> None:
    """Start the health check server."""
    server = HTTPServer(("", port), HealthHandler)
    server.serve_forever()
