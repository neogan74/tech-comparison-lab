CREATE KEYSPACE IF NOT EXISTS bench
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

CREATE TABLE IF NOT EXISTS bench.orders_by_country (
  country text,
  bucket int,
  id text,
  user_id text,
  user_tier text,
  product_id text,
  product_category text,
  product_price double,
  quantity int,
  status text,
  tags set<text>,
  metadata_source text,
  metadata_session text,
  created_at timestamp,
  updated_at timestamp,
  PRIMARY KEY ((country, bucket), id)
) WITH CLUSTERING ORDER BY (id ASC);
