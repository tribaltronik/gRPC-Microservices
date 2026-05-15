import json
import os
import subprocess
import time

import pytest

GRPCURL = os.path.expanduser("~/go/bin/grpcurl")
PROJECT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
CERTS_DIR = os.path.join(PROJECT_DIR, "certs")

USER_SERVICE_PORT = os.getenv("USER_SERVICE_PORT", "50051")
ORDER_SERVICE_PORT = os.getenv("ORDER_SERVICE_PORT", "50052")
ENVOY_PORT = os.getenv("ENVOY_PORT", "8080")
API_KEY = os.getenv("API_KEY", "grpc-poc-api-key-2026")

_SESSION_EMAIL_COUNTER = 0


def grpc_call(service, method, payload=None, port=USER_SERVICE_PORT,
              cert_dir="api-gateway"):
    cmd = [
        GRPCURL, "-insecure",
        "-cacert", os.path.join(CERTS_DIR, "ca", "ca.pem"),
        "-cert", os.path.join(CERTS_DIR, cert_dir, "cert.pem"),
        "-key", os.path.join(CERTS_DIR, cert_dir, "key.pem"),
    ]
    if payload is not None:
        cmd.extend(["-d", json.dumps(payload)])
    cmd.extend([f"localhost:{port}", f"{service}/{method}"])

    result = subprocess.run(cmd, capture_output=True, text=True)

    if result.returncode != 0:
        code, msg = "", ""
        for line in result.stderr.split("\n"):
            line = line.strip()
            if line.startswith("Code:"):
                code = line.split(":", 1)[1].strip()
            if line.startswith("Message:"):
                msg = line.split(":", 1)[1].strip()
        return None, code, msg

    if not result.stdout.strip():
        return {}, None, None

    return json.loads(result.stdout), None, None


def unique_email(prefix="test"):
    global _SESSION_EMAIL_COUNTER
    _SESSION_EMAIL_COUNTER += 1
    ts = int(time.time() * 1000)
    return f"{prefix}_{ts}_{_SESSION_EMAIL_COUNTER}@example.com"


@pytest.fixture(scope="session")
def api_key():
    return API_KEY


@pytest.fixture(scope="session")
def envoy_url():
    return f"http://localhost:{ENVOY_PORT}"


@pytest.fixture(scope="session")
def user_id():
    email = unique_email("fixture_user")
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "Integration Test User", "email": email},
    )
    assert code is None, f"Failed to create test user: {msg}"
    return data["user"]["id"]


@pytest.fixture(scope="session")
def second_user_id():
    email = unique_email("fixture_user2")
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "Second Test User", "email": email},
    )
    assert code is None, f"Failed to create second user: {msg}"
    return data["user"]["id"]


@pytest.fixture(scope="session")
def order_id(user_id):
    data, code, msg = grpc_call(
        "order.v1.OrderService", "CreateOrder",
        {
            "user_id": user_id,
            "items": [{
                "product_id": "fixture-prod",
                "product_name": "Fixture Product",
                "quantity": 1,
                "unit_price": 10.0,
            }],
        },
        port=ORDER_SERVICE_PORT,
    )
    assert code is None, f"Failed to create order: {msg}"
    return data["order"]["id"]
