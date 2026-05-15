import os
import subprocess

from conftest import CERTS_DIR, GRPCURL, PROJECT_DIR, grpc_call, unique_email


class TestMTLS:

    def test_valid_api_gateway_cert(self, unique_email):
        payload = {"name": "mTLS Test", "email": unique_email()}
        data, code, msg = grpc_call("user.v1.UserService", "CreateUser", payload)
        assert code is None, f"Expected success, got code={code} msg={msg}"
        assert data is not None
        assert "user" in data

    def test_user_service_cert_as_client(self, unique_email):
        payload = {"name": "mTLS Test 2", "email": unique_email()}
        data, code, msg = grpc_call(
            "user.v1.UserService", "CreateUser", payload,
            cert_dir="user-service",
        )
        if code is not None:
            assert code == "Unavailable"
        else:
            assert data is not None

    def test_missing_client_cert(self):
        cmd = [
            GRPCURL, "-insecure",
            "-cacert", os.path.join(CERTS_DIR, "ca", "ca.pem"),
            "localhost:50051",
            "user.v1.UserService/ListServices",
        ]
        result = subprocess.run(cmd, capture_output=True, text=True)
        assert result.returncode != 0
