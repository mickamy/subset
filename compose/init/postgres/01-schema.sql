CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped', 'cancelled');

CREATE TABLE tenants (
    id         serial      PRIMARY KEY,
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id         serial      PRIMARY KEY,
    tenant_id  int         NOT NULL REFERENCES tenants(id),
    email      text        NOT NULL,
    name       text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email)
);

CREATE TABLE products (
    id         serial      PRIMARY KEY,
    tenant_id  int         NOT NULL REFERENCES tenants(id),
    sku        text        NOT NULL,
    name       text        NOT NULL,
    price_yen  int         NOT NULL,
    UNIQUE (tenant_id, sku)
);

CREATE TABLE orders (
    id         serial       PRIMARY KEY,
    user_id    int          NOT NULL REFERENCES users(id),
    status     order_status NOT NULL,
    total_yen  int          NOT NULL,
    created_at timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE order_items (
    id         serial PRIMARY KEY,
    order_id   int    NOT NULL REFERENCES orders(id),
    product_id int    NOT NULL REFERENCES products(id),
    qty        int    NOT NULL,
    price_yen  int    NOT NULL
);
