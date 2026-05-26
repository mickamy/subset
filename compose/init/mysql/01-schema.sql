CREATE TABLE tenants (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE users (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    tenant_id  INT          NOT NULL,
    email      VARCHAR(255) NOT NULL,
    name       VARCHAR(255),
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, email),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE TABLE products (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    tenant_id  INT          NOT NULL,
    sku        VARCHAR(255) NOT NULL,
    name       VARCHAR(255) NOT NULL,
    price_yen  INT          NOT NULL,
    UNIQUE (tenant_id, sku),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE TABLE orders (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    user_id    INT       NOT NULL,
    status     ENUM('pending', 'paid', 'shipped', 'cancelled') NOT NULL,
    total_yen  INT       NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE order_items (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    order_id   INT NOT NULL,
    product_id INT NOT NULL,
    qty        INT NOT NULL,
    price_yen  INT NOT NULL,
    FOREIGN KEY (order_id)   REFERENCES orders(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
);
