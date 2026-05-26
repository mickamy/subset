INSERT INTO tenants (id, name) VALUES
    (1, 'Acme Inc'),
    (2, 'Beta Corp');

INSERT INTO users (id, tenant_id, email, name) VALUES
    (1, 1, 'alice@acme.com', 'Alice'),
    (2, 1, 'bob@acme.com',   'Bob'),
    (3, 1, 'carol@acme.com', 'Carol'),
    (4, 2, 'dave@beta.com',  'Dave'),
    (5, 2, 'eve@beta.com',   'Eve');

INSERT INTO products (id, tenant_id, sku, name, price_yen) VALUES
    (1, 1, 'ACME-001', 'Widget',      1000),
    (2, 1, 'ACME-002', 'Gadget',      1500),
    (3, 1, 'ACME-003', 'Gizmo',       2000),
    (4, 2, 'BETA-001', 'Doohickey',    800),
    (5, 2, 'BETA-002', 'Thingamajig', 1200);

INSERT INTO orders (id, user_id, status, total_yen) VALUES
    (1, 1, 'paid',    2500),
    (2, 1, 'pending', 1000),
    (3, 2, 'shipped', 3500),
    (4, 4, 'paid',    1200);

INSERT INTO order_items (id, order_id, product_id, qty, price_yen) VALUES
    (1, 1, 1, 1, 1000),
    (2, 1, 2, 1, 1500),
    (3, 2, 1, 1, 1000),
    (4, 3, 3, 1, 2000),
    (5, 3, 2, 1, 1500),
    (6, 4, 5, 1, 1200);
