import pytest
from conftest import grpc_call, unique_email


def test_create_user():
    email = unique_email()
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "Alice", "email": email},
    )
    assert code is None, msg
    user = data["user"]
    assert user["id"]
    assert user["name"] == "Alice"
    assert user["email"] == email
    assert user["createdAt"]
    assert user["updatedAt"]


def test_get_user(user_id):
    data, code, msg = grpc_call("user.v1.UserService", "GetUser", {"id": user_id})
    assert code is None, msg
    assert data["user"]["name"] == "Integration Test User"


def test_update_user_name(user_id):
    orig, code, msg = grpc_call("user.v1.UserService", "GetUser", {"id": user_id})
    assert code is None, msg
    orig_email = orig["user"]["email"]

    new_name = "Updated Name"
    data, code, msg = grpc_call(
        "user.v1.UserService", "UpdateUser",
        {"id": user_id, "user": {"name": new_name}, "update_mask": {"paths": ["name"]}},
    )
    assert code is None, msg
    user = data["user"]
    assert user["name"] == new_name
    assert user["email"] == orig_email


def test_update_user_email(user_id):
    orig, code, msg = grpc_call("user.v1.UserService", "GetUser", {"id": user_id})
    assert code is None, msg
    orig_name = orig["user"]["name"]

    new_email = unique_email("new_email")
    data, code, msg = grpc_call(
        "user.v1.UserService", "UpdateUser",
        {"id": user_id, "user": {"email": new_email}, "update_mask": {"paths": ["email"]}},
    )
    assert code is None, msg
    user = data["user"]
    assert user["email"] == new_email
    assert user["name"] == orig_name


def test_update_user_both(user_id):
    new_name = "Both Updated"
    new_email = unique_email("both")
    data, code, msg = grpc_call(
        "user.v1.UserService", "UpdateUser",
        {
            "id": user_id,
            "user": {"name": new_name, "email": new_email},
            "update_mask": {"paths": ["name", "email"]},
        },
    )
    assert code is None, msg
    user = data["user"]
    assert user["name"] == new_name
    assert user["email"] == new_email


def test_delete_user():
    email = unique_email()
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "Delete Me", "email": email},
    )
    assert code is None, msg
    uid = data["user"]["id"]

    data, code, msg = grpc_call("user.v1.UserService", "DeleteUser", {"id": uid})
    assert code is None, msg
    assert data["message"]


def test_user_not_found_after_delete():
    email = unique_email()
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "Temp User", "email": email},
    )
    assert code is None, msg
    uid = data["user"]["id"]

    data, code, msg = grpc_call("user.v1.UserService", "DeleteUser", {"id": uid})
    assert code is None, msg

    data, code, msg = grpc_call("user.v1.UserService", "GetUser", {"id": uid})
    assert code == "NotFound"


def test_create_user_empty_name():
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "", "email": unique_email()},
    )
    assert code == "InvalidArgument"
    assert "name" in msg.lower()


def test_create_user_missing_at():
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "Test", "email": "noat"},
    )
    assert code == "InvalidArgument"
    assert "email" in msg.lower()


def test_create_user_duplicate_email():
    email = unique_email()
    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "First", "email": email},
    )
    assert code is None, msg

    data, code, msg = grpc_call(
        "user.v1.UserService", "CreateUser",
        {"name": "Second", "email": email},
    )
    assert code == "AlreadyExists"


def test_get_user_empty_id():
    data, code, msg = grpc_call("user.v1.UserService", "GetUser", {"id": ""})
    assert code == "InvalidArgument"


def test_get_user_not_found():
    data, code, msg = grpc_call(
        "user.v1.UserService", "GetUser",
        {"id": "00000000-0000-0000-0000-000000000000"},
    )
    assert code == "NotFound"


def test_update_user_empty_mask(user_id):
    data, code, msg = grpc_call(
        "user.v1.UserService", "UpdateUser",
        {"id": user_id, "user": {"name": "x"}, "update_mask": {"paths": []}},
    )
    assert code == "InvalidArgument"


def test_update_user_unknown_field(user_id):
    data, code, msg = grpc_call(
        "user.v1.UserService", "UpdateUser",
        {"id": user_id, "user": {"name": "x"}, "update_mask": {"paths": ["phone"]}},
    )
    assert code == "InvalidArgument"
    assert "phone" in msg.lower()


def test_update_user_not_found():
    data, code, msg = grpc_call(
        "user.v1.UserService", "UpdateUser",
        {
            "id": "00000000-0000-0000-0000-000000000000",
            "user": {"name": "x"},
            "update_mask": {"paths": ["name"]},
        },
    )
    assert code == "NotFound"


def test_delete_user_empty_id():
    data, code, msg = grpc_call("user.v1.UserService", "DeleteUser", {"id": ""})
    assert code == "InvalidArgument"


def test_delete_user_not_found():
    data, code, msg = grpc_call(
        "user.v1.UserService", "DeleteUser",
        {"id": "00000000-0000-0000-0000-000000000000"},
    )
    assert code == "NotFound"
