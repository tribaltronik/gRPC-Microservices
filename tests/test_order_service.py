import pytest
from conftest import grpc_call, unique_email

ORDER_SERVICE = "order.v1.OrderService"
ORDER_PORT = "50052"


def test_create_order(user_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CreateOrder",
        {
            "userId": user_id,
            "items": [{
                "productId": "prod-1",
                "productName": "Test Product",
                "quantity": 2,
                "unitPrice": 15.50,
            }],
        },
        port=ORDER_PORT,
    )
    assert code is None, f"CreateOrder failed: {msg}"
    order = data["order"]
    assert order["id"] is not None
    assert order["total"] == 31.0
    assert order["status"] == "ORDER_STATUS_PENDING"
    assert len(order["items"]) == 1
    assert order["items"][0]["productId"] == "prod-1"
    assert order["items"][0]["quantity"] == 2


def test_get_order(order_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "GetOrder",
        {"id": order_id},
        port=ORDER_PORT,
    )
    assert code is None, f"GetOrder failed: {msg}"
    order = data["order"]
    assert order["id"] == order_id
    assert "userId" in order
    assert "total" in order
    assert "status" in order
    assert "items" in order
    assert "createdAt" in order


def test_list_orders():
    data, code, msg = grpc_call(
        ORDER_SERVICE, "ListOrders",
        {},
        port=ORDER_PORT,
    )
    assert code is None, f"ListOrders failed: {msg}"
    assert "orders" in data
    assert "pagination" in data
    pag = data["pagination"]
    assert "totalCount" in pag
    assert pag["totalCount"] >= 0


def test_list_orders_by_user(user_id, second_user_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "ListOrders",
        {"userId": user_id},
        port=ORDER_PORT,
    )
    assert code is None, f"ListOrders by user failed: {msg}"
    for order in data["orders"]:
        assert order["userId"] == user_id


def test_list_orders_pagination():
    data, code, msg = grpc_call(
        ORDER_SERVICE, "ListOrders",
        {"pagination": {"page": 1, "pageSize": 1}},
        port=ORDER_PORT,
    )
    assert code is None, f"ListOrders pagination failed: {msg}"
    assert len(data["orders"]) == 1
    assert data["pagination"]["hasMore"] is True


def test_cancel_order(order_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CancelOrder",
        {"id": order_id},
        port=ORDER_PORT,
    )
    assert code is None, f"CancelOrder failed: {msg}"
    assert data["order"]["status"] == "ORDER_STATUS_CANCELLED"


def test_cancel_order_twice(order_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CancelOrder",
        {"id": order_id},
        port=ORDER_PORT,
    )
    assert code is None, f"First cancel failed: {msg}"

    data, code, msg = grpc_call(
        ORDER_SERVICE, "CancelOrder",
        {"id": order_id},
        port=ORDER_PORT,
    )
    assert code is None, f"Second cancel failed: {msg}"
    assert data["order"]["status"] == "ORDER_STATUS_CANCELLED"


def test_order_full_lifecycle(user_id):
    items = [{"productId": "lifecycle-prod", "productName": "Lifecycle Product", "quantity": 1, "unitPrice": 5.0}]
    data, code, msg = grpc_call(ORDER_SERVICE, "CreateOrder", {"userId": user_id, "items": items}, port=ORDER_PORT)
    assert code is None, f"CreateOrder failed: {msg}"
    order_id = data["order"]["id"]

    data, code, msg = grpc_call(ORDER_SERVICE, "GetOrder", {"id": order_id}, port=ORDER_PORT)
    assert code is None, f"GetOrder failed: {msg}"
    assert data["order"]["status"] == "ORDER_STATUS_PENDING"

    data, code, msg = grpc_call(ORDER_SERVICE, "CancelOrder", {"id": order_id}, port=ORDER_PORT)
    assert code is None, f"CancelOrder failed: {msg}"
    assert data["order"]["status"] == "ORDER_STATUS_CANCELLED"

    data, code, msg = grpc_call(ORDER_SERVICE, "GetOrder", {"id": order_id}, port=ORDER_PORT)
    assert code is None, f"GetOrder after cancel failed: {msg}"
    assert data["order"]["status"] == "ORDER_STATUS_CANCELLED"


def test_create_order_missing_user_id():
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CreateOrder",
        {"userId": "", "items": [{"productId": "p1", "productName": "P", "quantity": 1, "unitPrice": 1.0}]},
        port=ORDER_PORT,
    )
    assert code is not None, "Expected error for empty user_id"
    assert "InvalidArgument" in code or "INVALID_ARGUMENT" in code


def test_create_order_empty_items(user_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CreateOrder",
        {"userId": user_id, "items": []},
        port=ORDER_PORT,
    )
    assert code is not None, "Expected error for empty items"
    assert "InvalidArgument" in code or "INVALID_ARGUMENT" in code


def test_create_order_missing_product_id(user_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CreateOrder",
        {"userId": user_id, "items": [{"productId": "", "productName": "P", "quantity": 1, "unitPrice": 1.0}]},
        port=ORDER_PORT,
    )
    assert code is not None, "Expected error for empty product_id"
    assert "InvalidArgument" in code or "INVALID_ARGUMENT" in code


def test_create_order_zero_quantity(user_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CreateOrder",
        {"userId": user_id, "items": [{"productId": "p1", "productName": "P", "quantity": 0, "unitPrice": 1.0}]},
        port=ORDER_PORT,
    )
    assert code is not None, "Expected error for zero quantity"
    assert "InvalidArgument" in code or "INVALID_ARGUMENT" in code


def test_create_order_negative_price(user_id):
    data, code, msg = grpc_call(
        ORDER_SERVICE, "CreateOrder",
        {"userId": user_id, "items": [{"productId": "p1", "productName": "P", "quantity": 1, "unitPrice": -1}]},
        port=ORDER_PORT,
    )
    assert code is not None, "Expected error for negative price"
    assert "InvalidArgument" in code or "INVALID_ARGUMENT" in code


def test_get_order_empty_id():
    data, code, msg = grpc_call(ORDER_SERVICE, "GetOrder", {"id": ""}, port=ORDER_PORT)
    assert code is not None, "Expected error for empty id"
    assert "InvalidArgument" in code or "INVALID_ARGUMENT" in code


def test_get_order_not_found():
    data, code, msg = grpc_call(ORDER_SERVICE, "GetOrder", {"id": "00000000-0000-0000-0000-000000000000"}, port=ORDER_PORT)
    assert code is not None, "Expected NotFound error"
    assert "NotFound" in code or "NOT_FOUND" in code


def test_cancel_order_empty_id():
    data, code, msg = grpc_call(ORDER_SERVICE, "CancelOrder", {"id": ""}, port=ORDER_PORT)
    assert code is not None, "Expected error for empty id"
    assert "InvalidArgument" in code or "INVALID_ARGUMENT" in code


def test_cancel_order_not_found():
    data, code, msg = grpc_call(ORDER_SERVICE, "CancelOrder", {"id": "00000000-0000-0000-0000-000000000000"}, port=ORDER_PORT)
    assert code is not None, "Expected NotFound error"
    assert "NotFound" in code or "NOT_FOUND" in code


def test_cross_service_flow():
    email = unique_email("cross_svc")
    data, code, msg = grpc_call("user.v1.UserService", "CreateUser", {"name": "Cross Svc User", "email": email})
    assert code is None, f"CreateUser failed: {msg}"
    new_user_id = data["user"]["id"]

    items = [{"productId": "cross-prod", "productName": "Cross Product", "quantity": 1, "unitPrice": 25.0}]
    data, code, msg = grpc_call(ORDER_SERVICE, "CreateOrder", {"userId": new_user_id, "items": items}, port=ORDER_PORT)
    assert code is None, f"CreateOrder failed: {msg}"
    order_id = data["order"]["id"]

    data, code, msg = grpc_call(ORDER_SERVICE, "GetOrder", {"id": order_id}, port=ORDER_PORT)
    assert code is None, f"GetOrder failed: {msg}"
    assert data["order"]["userId"] == new_user_id
