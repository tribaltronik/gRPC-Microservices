import subprocess
import time

from conftest import PROJECT_DIR, grpc_call, unique_email


def docker_compose(*args):
    return subprocess.run(
        ["docker", "compose"] + list(args),
        capture_output=True, text=True,
        cwd=PROJECT_DIR,
    )


class TestResilience:

    def test_db_stop_and_recover(self, unique_email):
        email = unique_email("resilience")
        payload = {"name": "Resilience Test", "email": email}

        data, code, msg = grpc_call("user.v1.UserService", "CreateUser", payload)
        assert code is None, f"Baseline call failed: {msg}"

        docker_compose("stop", "user-db")
        time.sleep(3)

        data, code, msg = grpc_call("user.v1.UserService", "CreateUser", payload)
        assert code == "Internal", f"Expected Internal after DB stop, got {code}: {msg}"

        docker_compose("start", "user-db")
        time.sleep(10)

        data, code, msg = grpc_call("user.v1.UserService", "CreateUser", payload)
        assert code is None, f"Recovery call failed: {msg}"
