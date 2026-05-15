import requests


class TestEnvoyAuth:

    def test_missing_api_key(self, envoy_url):
        resp = requests.post(f"{envoy_url}/api/v1/users")
        assert resp.status_code == 401

    def test_invalid_api_key(self, envoy_url):
        headers = {"x-api-key": "wrong"}
        resp = requests.post(f"{envoy_url}/api/v1/users", headers=headers)
        assert resp.status_code == 401

    def test_valid_api_key(self, envoy_url, api_key, unique_email):
        headers = {"x-api-key": api_key}
        body = {"name": "AuthTest", "email": unique_email()}
        resp = requests.post(f"{envoy_url}/api/v1/users", headers=headers, json=body)
        assert resp.status_code != 401

    def test_auth_error_response_body(self, envoy_url):
        resp = requests.post(f"{envoy_url}/api/v1/users")
        assert resp.status_code == 401
        assert resp.text
