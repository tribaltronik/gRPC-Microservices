import pytest
from conftest import grpc_call, unique_email

USER_SERVICE = "user.v1.UserService"
ORDER_SERVICE = "order.v1.OrderService"
ORDER_PORT = "50052"


def test_list_orders_negative_page():
    data, code, msg = grpc_call(
        ORDER_SERVICE, "ListOrders",
        {"pagination": {"page": -1, "page_size": 10}},
        port=ORDER_PORT,
    )
    assert code is None, f"Expected success with defaulted page, got {code}: {msg}"
    assert data["pagination"]["totalCount"] >= 0


def test_list_orders_large_page_size():
    data, code, msg = grpc_call(
        ORDER_SERVICE, "ListOrders",
        {"pagination": {"page": 1, "page_size": 1000}},
        port=ORDER_PORT,
    )
    assert code is None, f"Expected success with clamped page_size, got {code}: {msg}"
    assert len(data.get("orders", [])) <= 100


def test_update_user_empty_name(user_id):
    data, code, msg = grpc_call(USER_SERVICE, "UpdateUser", {
        "id": user_id,
        "user": {"name": ""},
        "updateMask": {"paths": ["name"]},
    })
    assert code == "InvalidArgument", f"Expected error for empty name, got: {msg}"
    assert "name" in msg.lower()


def test_update_user_empty_email(user_id):
    data, code, msg = grpc_call(USER_SERVICE, "UpdateUser", {
        "id": user_id,
        "user": {"email": ""},
        "updateMask": {"paths": ["email"]},
    })
    assert code == "InvalidArgument", f"Expected error for empty email, got: {msg}"
    assert "email" in msg.lower()


def test_update_user_valid_uppercase_email(user_id):
    email = unique_email("upper")
    data, code, msg = grpc_call(USER_SERVICE, "UpdateUser", {
        "id": user_id,
        "user": {"email": email.upper()},
        "updateMask": {"paths": ["email"]},
    })
    assert code is None, f"UpdateUser failed: {msg}"
    assert data["user"]["email"] == email.upper(), "Email should preserve case"


def test_create_user_special_chars():
    email = unique_email("special_chars")
    data, code, msg = grpc_call(
        USER_SERVICE, "CreateUser",
        {"name": "O'Brien & Smith (Test/Dev)", "email": email},
    )
    assert code is None, f"CreateUser with special chars failed: {msg}"
    assert "O'Brien" in data["user"]["name"]
