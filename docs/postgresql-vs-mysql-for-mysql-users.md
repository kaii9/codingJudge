# PostgreSQL vs MySQL 对比学习：给熟悉 MySQL 的后端工程师

> 目标读者：已经熟练使用 MySQL/InnoDB，想系统理解 PostgreSQL 在原理、SQL 使用、索引、事务、锁、执行计划和工程实践上的差异。
>
> 阅读目标：不是背“谁更好”，而是能在面试和项目设计中解释：同一个后端需求在 MySQL 和 PostgreSQL 下为什么写法不同、锁行为不同、索引设计不同、故障风险不同。

## 1. 先给结论

如果你已经会 MySQL，可以把 PostgreSQL 理解成：

```text
MySQL/InnoDB：存储引擎主导，B+Tree 聚簇索引心智很重要
PostgreSQL：数据库内核整体主导，MVCC tuple、丰富类型、表达式能力很重要
```

两个数据库都能支撑常见后端系统，但关注点不同：

| 维度 | MySQL/InnoDB | PostgreSQL |
| --- | --- | --- |
| 默认存储模型 | InnoDB 聚簇索引表 | Heap table + 独立索引 |
| MVCC 清理 | undo log + purge | 表内多版本 tuple + vacuum |
| 默认隔离级别 | REPEATABLE READ | READ COMMITTED |
| SQL 能力 | 工程常用够用，方言偏实用 | SQL 表达能力强，类型和函数丰富 |
| JSON | JSON 类型，函数逐步增强 | json/jsonb，索引和操作符更强 |
| 索引生态 | B+Tree 为主，全文/空间等 | B-tree、GIN、GiST、BRIN、Hash、表达式、部分索引 |
| UPSERT | `ON DUPLICATE KEY UPDATE` | `ON CONFLICT DO UPDATE` |
| 分页常见写法 | `LIMIT offset, size` | `LIMIT size OFFSET offset` |
| 自增 | `AUTO_INCREMENT` | `IDENTITY` / sequence |
| 运维关键词 | buffer pool、binlog、redo/undo、主从 | shared buffers、WAL、vacuum、autovacuum |

最重要的一句话：

> MySQL 的核心心智是“聚簇索引 + undo/redo + next-key lock”；PostgreSQL 的核心心智是“heap tuple 多版本 + WAL + vacuum + 强类型/强 SQL 表达”。

## 2. 架构层面的根本差异

### 2.1 MySQL 是 Server + Storage Engine 架构

MySQL 的经典架构分层：

```text
Client
  ↓
MySQL Server
  - parser
  - optimizer
  - executor
  - privilege
  ↓
Storage Engine API
  ↓
InnoDB / MyISAM / Memory / ...
```

实际后端开发中，你几乎总是在用 InnoDB，所以很多 MySQL 行为其实是 InnoDB 行为：

- 事务；
- 行锁；
- MVCC；
- 外键；
- redo log；
- undo log；
- buffer pool；
- 聚簇索引。

所以面试中说“MySQL 默认支持事务”要更严谨：

> MySQL Server 本身支持多存储引擎，事务语义主要来自 InnoDB。

### 2.2 PostgreSQL 是更一体化的数据库内核

PostgreSQL 没有 MySQL 那种常见的可插拔存储引擎心智。它的表、索引、事务、WAL、MVCC、vacuum 都是 PostgreSQL 内核整体设计的一部分。

这带来两个直观结果：

1. PostgreSQL 的 SQL 类型、函数、索引、约束、查询能力更整体；
2. PostgreSQL 的 MVCC 清理、膨胀、vacuum 是你必须理解的运维概念。

如果你从 MySQL 转 PostgreSQL，不要只找“等价语法”，要理解底层模型变了。

## 3. 表存储模型：聚簇索引 vs Heap Table

### 3.1 MySQL/InnoDB：表数据按主键聚簇

InnoDB 表是按主键组织的 B+Tree：

```text
PRIMARY KEY B+Tree leaf
  → 存整行数据

Secondary Index leaf
  → 存二级索引列 + 主键值
  → 再回表到主键 B+Tree
```

这就是你熟悉的：

- 主键越短越好；
- 二级索引会带上主键；
- 用二级索引查非覆盖列需要回表；
- 主键顺序插入通常更友好；
- 随机 UUID 主键可能导致页分裂和写入放大。

示例：

```sql
CREATE TABLE submissions (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  problem_id BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  INDEX idx_user_created (user_id, created_at)
) ENGINE=InnoDB;
```

`idx_user_created` 的叶子节点大致保存：

```text
user_id, created_at, primary_key_id
```

如果查询列不在二级索引中，就通过 primary key 回表。

### 3.2 PostgreSQL：Heap Table + 索引指向 tuple 位置

PostgreSQL 默认表是 heap table。索引不存整行，也不是聚簇表本身。索引项通常指向 heap 中的 tuple 位置。

```text
Heap table
  → 存行版本 tuple

B-tree index
  → key + tuple pointer
```

这意味着：

- PostgreSQL 主键本质上是一个唯一 B-tree 索引；
- 表数据不天然按主键物理聚簇；
- 可以用 `CLUSTER` 按某个索引重排表，但不是自动持续维护；
- 更新行时可能生成新 tuple 版本，索引是否更新取决于 HOT update 等条件。

PostgreSQL 示例：

```sql
CREATE TABLE submissions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id BIGINT NOT NULL,
  problem_id BIGINT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX submissions_user_created_idx
  ON submissions (user_id, created_at DESC);
```

如果你从 MySQL 迁移，第一件事是放下“主键就是表物理组织方式”的默认假设。

## 4. MVCC：Undo Log vs Tuple 多版本

MVCC 是两个数据库都很重要的概念，但实现差异很大。

### 4.1 MySQL/InnoDB MVCC

InnoDB 的 MVCC 依赖：

- 当前数据页中的最新行；
- undo log 保存旧版本；
- Read View 判断哪些版本对当前事务可见。

简化理解：

```text
当前行记录
  → roll pointer
  → undo log 旧版本
  → 更旧版本
```

普通一致性读不会加锁，而是通过 Read View 找到可见版本。

典型影响：

- 长事务会阻止 undo purge；
- undo log 堆积会影响空间和性能；
- REPEATABLE READ 下同一事务多次快照读看到一致结果；
- 当前读如 `SELECT ... FOR UPDATE` 会读取最新并加锁。

### 4.2 PostgreSQL MVCC

PostgreSQL 把行版本直接放在表 heap 中。更新不是原地覆盖，而是插入新 tuple 版本，旧版本暂时保留。

每个 tuple 有版本可见性元信息，典型包括：

```text
xmin：创建该版本的事务
xmax：删除或更新该版本的事务
```

简化：

```text
old tuple version
new tuple version
index / heap visibility
vacuum later cleans dead tuple
```

典型影响：

- 更新和删除会产生 dead tuple；
- autovacuum 很重要；
- 表膨胀和索引膨胀是 PostgreSQL 需要关注的运维问题；
- 长事务会阻止 dead tuple 清理；
- `VACUUM` 不是“可有可无”的维护命令，而是 MVCC 生命周期的一部分。

### 4.3 面试对比说法

可以这样回答：

> MySQL/InnoDB 通过 undo log 保存旧版本，PostgreSQL 则把多个 tuple 版本留在表里，再由 vacuum 清理。两者都能实现 MVCC，但 MySQL 更常关注 undo/purge，PostgreSQL 更常关注 autovacuum、dead tuple 和 table bloat。

## 5. 事务隔离级别和锁

### 5.1 默认隔离级别不同

| 数据库 | 默认隔离级别 |
| --- | --- |
| MySQL/InnoDB | REPEATABLE READ |
| PostgreSQL | READ COMMITTED |

这会影响你对“同一事务内重复查询是否看到新提交数据”的预期。

MySQL 默认：

```sql
START TRANSACTION;
SELECT * FROM orders WHERE id = 1;
-- 其他事务提交修改
SELECT * FROM orders WHERE id = 1;
COMMIT;
```

在普通快照读下，同一事务内通常看到同一个 Read View。

PostgreSQL 默认 READ COMMITTED：

```sql
BEGIN;
SELECT * FROM orders WHERE id = 1;
-- 其他事务提交修改
SELECT * FROM orders WHERE id = 1;
COMMIT;
```

每条语句拿一个新快照，第二次查询可能看到其他事务刚提交的数据。

### 5.2 MySQL 的 next-key lock

InnoDB 在 REPEATABLE READ 下为了防止幻读，常使用 next-key lock：

```text
record lock + gap lock
```

例如：

```sql
SELECT *
FROM orders
WHERE user_id = 10 AND amount > 100
FOR UPDATE;
```

如果命中索引范围，InnoDB 可能锁住记录和索引间隙，影响其他事务插入范围内的新行。

这就是 MySQL 面试常问的：

- record lock；
- gap lock；
- next-key lock；
- 当前读；
- 快照读；
- 唯一索引等值查询是否退化为 record lock。

### 5.3 PostgreSQL 的锁心智

PostgreSQL 也支持行锁：

```sql
SELECT *
FROM orders
WHERE id = 1
FOR UPDATE;
```

但它没有 InnoDB 那套完全相同的 gap lock 心智。PostgreSQL 在 SERIALIZABLE 下使用 SSI 等机制处理序列化异常，而不是让你用 InnoDB next-key lock 的方式推理所有范围锁。

PostgreSQL 常见行锁语法更丰富：

```sql
FOR UPDATE
FOR NO KEY UPDATE
FOR SHARE
FOR KEY SHARE
SKIP LOCKED
NOWAIT
```

例如任务队列抢占：

```sql
SELECT id
FROM judge_outbox
WHERE published_at IS NULL
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 10;
```

MySQL 8 也支持 `SKIP LOCKED`，但在默认隔离级别、索引和 gap lock 影响下，需要重新测试锁范围。

### 5.4 GoJudge 中的意义

GoJudge 的可靠 Worker 用 PostgreSQL 实现：

- outbox 抢占；
- submission 行锁认领；
- lease；
- fencing token 条件更新。

PostgreSQL 写法可以比较紧凑：

```sql
WITH candidates AS (
  SELECT id
  FROM judge_outbox
  WHERE published_at IS NULL
  ORDER BY id
  FOR UPDATE SKIP LOCKED
  LIMIT $1
)
UPDATE judge_outbox AS outbox
SET claimed_by = $2
FROM candidates
WHERE outbox.id = candidates.id
RETURNING outbox.id;
```

MySQL 8 可以实现同等业务语义，但通常会拆成事务内多步：

```sql
START TRANSACTION;

SELECT id
FROM judge_outbox
WHERE published_at IS NULL
ORDER BY id
LIMIT ?
FOR UPDATE SKIP LOCKED;

UPDATE judge_outbox
SET claimed_by = ?, claim_expires_at = ?
WHERE id IN (...);

SELECT id, submission_id
FROM judge_outbox
WHERE claimed_by = ?;

COMMIT;
```

结论：

> PostgreSQL 不是“能做 MySQL 做不了的事”，而是很多事务化数据修改可以写得更接近 SQL 表达本身；MySQL 也能做，但通常需要更明确地拆事务步骤并验证锁行为。

## 6. 索引体系差异

### 6.1 B-tree：两者都最常用

MySQL 和 PostgreSQL 都大量使用 B-tree 索引，适合：

- 等值查询；
- 范围查询；
- 排序；
- 前缀匹配；
- 联合索引最左前缀。

MySQL 示例：

```sql
CREATE INDEX idx_user_status_created
ON submissions(user_id, status, created_at);
```

PostgreSQL 示例：

```sql
CREATE INDEX submissions_user_status_created_idx
ON submissions(user_id, status, created_at DESC);
```

共同原则：

- 高选择性列适合放索引；
- 联合索引顺序要服务查询条件和排序；
- 低选择性字段单独建索引收益有限；
- 过多索引会拖慢写入。

### 6.2 PostgreSQL 部分索引

PostgreSQL 支持 partial index：

```sql
CREATE INDEX judge_outbox_publish_idx
ON judge_outbox(next_attempt_at, id)
WHERE published_at IS NULL;
```

这表示只索引未发布行。

适合：

- 只查少量活跃状态；
- 绝大多数历史行不参与查询；
- 队列、软删除、状态机表。

MySQL 没有直接等价的 partial index。常见替代：

1. 联合索引带状态列：

```sql
CREATE INDEX idx_outbox_published_next_id
ON judge_outbox(published_at, next_attempt_at, id);
```

2. 生成列模拟条件：

```sql
ALTER TABLE judge_outbox
ADD COLUMN unpublished TINYINT
  AS (published_at IS NULL) STORED,
ADD INDEX idx_unpublished_next_id(unpublished, next_attempt_at, id);
```

### 6.3 PostgreSQL 表达式索引

PostgreSQL 可以直接对表达式建索引：

```sql
CREATE INDEX users_lower_username_idx
ON users (lower(username));
```

然后查询：

```sql
SELECT *
FROM users
WHERE lower(username) = lower($1);
```

MySQL 8 可以通过函数索引或生成列实现类似能力，但工程上经常使用归一化字段：

```sql
username_normalized VARCHAR(64) NOT NULL UNIQUE
```

GoJudge 采用 `username_normalized`，这对 PostgreSQL 和 MySQL 都友好。

### 6.4 GIN、GiST、BRIN

PostgreSQL 的索引类型更丰富：

| 索引 | 常见用途 |
| --- | --- |
| B-tree | 通用等值、范围、排序 |
| GIN | 数组、jsonb、全文检索 |
| GiST | 几何、范围类型、相似搜索 |
| BRIN | 大表按物理顺序相关字段的粗粒度索引 |
| Hash | 等值查询，使用少于 B-tree |

例如 jsonb：

```sql
CREATE INDEX problems_meta_gin_idx
ON problems
USING GIN (metadata);
```

MySQL 也支持 JSON 和全文索引，但 PostgreSQL 在复杂类型和索引组合上的表达能力更强。

## 7. SQL 方言差异

### 7.1 参数占位符

Go 里使用不同驱动时会遇到占位符差异：

| 数据库 | 常见占位符 |
| --- | --- |
| PostgreSQL pgx/libpq | `$1`, `$2` |
| MySQL driver | `?` |

PostgreSQL：

```go
row := db.QueryRow(ctx, `
  SELECT id, username
  FROM users
  WHERE id = $1
`, userID)
```

MySQL：

```go
row := db.QueryRowContext(ctx, `
  SELECT id, username
  FROM users
  WHERE id = ?
`, userID)
```

### 7.2 UPSERT

MySQL：

```sql
INSERT INTO users(username, password_hash)
VALUES (?, ?)
ON DUPLICATE KEY UPDATE
  password_hash = VALUES(password_hash);
```

PostgreSQL：

```sql
INSERT INTO users(username, password_hash)
VALUES ($1, $2)
ON CONFLICT (username)
DO UPDATE SET password_hash = EXCLUDED.password_hash;
```

PostgreSQL 的 `ON CONFLICT` 可以明确指定冲突目标，语义通常更清晰。

### 7.3 RETURNING

PostgreSQL 支持：

```sql
INSERT INTO submissions(problem_id, status)
VALUES ($1, 'queued')
RETURNING id, created_at;
```

MySQL 常见做法：

```sql
INSERT INTO submissions(problem_id, status)
VALUES (?, 'queued');

SELECT LAST_INSERT_ID();
```

或应用层生成 ID。

PostgreSQL 的 `UPDATE ... RETURNING` 也很常用：

```sql
UPDATE submissions
SET status = 'running'
WHERE id = $1 AND status = 'queued'
RETURNING id, status;
```

MySQL 需要拆成 `UPDATE` 后检查 affected rows，再 `SELECT`。

### 7.4 UPDATE FROM

PostgreSQL：

```sql
UPDATE judge_outbox AS o
SET claimed_by = $1
FROM candidates AS c
WHERE o.id = c.id;
```

MySQL：

```sql
UPDATE judge_outbox o
JOIN candidates c ON o.id = c.id
SET o.claimed_by = ?;
```

MySQL 也能 join update，但 CTE、锁和返回结果的组合能力需要重新设计。

### 7.5 LIMIT 写法

MySQL：

```sql
SELECT *
FROM submissions
ORDER BY created_at DESC
LIMIT 20, 10;
```

PostgreSQL：

```sql
SELECT *
FROM submissions
ORDER BY created_at DESC
LIMIT 10 OFFSET 20;
```

为了可读性，MySQL 也建议写：

```sql
LIMIT 10 OFFSET 20
```

### 7.6 字符串和大小写

MySQL 的大小写敏感性常受 collation 影响：

```sql
username VARCHAR(64) COLLATE utf8mb4_0900_ai_ci
```

`ci` 通常表示 case-insensitive。

PostgreSQL 默认文本比较通常是大小写敏感。常见做法：

```sql
CREATE UNIQUE INDEX users_lower_username_idx
ON users (lower(username));
```

或保存：

```sql
username_normalized TEXT NOT NULL UNIQUE
```

GoJudge 选择后者，便于跨数据库。

## 8. 数据类型差异

### 8.1 自增主键

MySQL：

```sql
id BIGINT PRIMARY KEY AUTO_INCREMENT
```

PostgreSQL：

```sql
id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY
```

老写法 `SERIAL` 仍常见，但新项目更推荐 identity。

### 8.2 时间类型

MySQL 常见：

```sql
created_at DATETIME(6) NOT NULL
```

PostgreSQL 常见：

```sql
created_at TIMESTAMPTZ NOT NULL DEFAULT now()
```

注意：

- PostgreSQL `TIMESTAMPTZ` 是带时区语义的时间点，内部规范化存储；
- MySQL `DATETIME` 不带时区，通常由应用约定 UTC；
- 跨库迁移时要统一“应用层全部 UTC”。

### 8.3 Boolean

PostgreSQL 有真正 boolean：

```sql
is_active BOOLEAN NOT NULL DEFAULT true
```

MySQL 常用：

```sql
is_active TINYINT(1) NOT NULL DEFAULT 1
```

MySQL 的 `BOOLEAN` 本质上通常是 `TINYINT(1)` 别名。

### 8.4 JSON

MySQL：

```sql
metadata JSON NOT NULL
```

PostgreSQL：

```sql
metadata JSONB NOT NULL
```

PostgreSQL `jsonb` 会解析成二进制格式，便于索引和查询：

```sql
SELECT *
FROM problems
WHERE metadata @> '{"difficulty":"hard"}';
```

MySQL：

```sql
SELECT *
FROM problems
WHERE JSON_EXTRACT(metadata, '$.difficulty') = 'hard';
```

复杂 JSON 查询和索引上，PostgreSQL 通常更舒服。

## 9. 执行计划与优化器

### 9.1 MySQL EXPLAIN

MySQL 你熟悉：

```sql
EXPLAIN SELECT *
FROM submissions
WHERE user_id = 1
ORDER BY created_at DESC
LIMIT 20;
```

重点看：

- type；
- key；
- rows；
- Extra；
- 是否 filesort；
- 是否 using index；
- 是否 using where。

### 9.2 PostgreSQL EXPLAIN

PostgreSQL 常用：

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT *
FROM submissions
WHERE user_id = 1
ORDER BY created_at DESC
LIMIT 20;
```

重点看：

- estimated cost；
- actual time；
- actual rows；
- loops；
- shared hit/read；
- scan 类型；
- filter 移除多少行；
- sort 是否落盘。

常见 scan：

| Scan | 含义 |
| --- | --- |
| Seq Scan | 全表扫描 |
| Index Scan | 走索引，再回 heap |
| Index Only Scan | 只走索引，依赖 visibility map |
| Bitmap Index Scan | 先从索引收集位置 |
| Bitmap Heap Scan | 再批量回表 |

MySQL 里你常说“回表”；PostgreSQL 里也有类似 heap fetch，但因为 heap table + visibility 检查，Index Only Scan 还依赖 visibility map。

### 9.3 统计信息

两个数据库都依赖统计信息。PostgreSQL 常见：

```sql
ANALYZE submissions;
```

MySQL：

```sql
ANALYZE TABLE submissions;
```

PostgreSQL 在复杂查询、join、表达式、分布估计上通常更依赖准确统计信息。数据倾斜时，执行计划可能和预期差别很大。

## 10. DDL 和迁移差异

### 10.1 MySQL DDL

MySQL 8/InnoDB 对 online DDL 有很多优化，但实际是否拷表、是否锁表取决于操作类型：

```sql
ALTER TABLE submissions
ADD COLUMN user_id BIGINT NULL,
ALGORITHM=INPLACE,
LOCK=NONE;
```

但不是所有 ALTER 都能无锁。

### 10.2 PostgreSQL DDL

PostgreSQL DDL 很多操作会拿较强锁。常见注意点：

- 大表加带默认值的列要谨慎；
- 建索引用 `CREATE INDEX CONCURRENTLY` 降低写阻塞；
- `CREATE INDEX CONCURRENTLY` 不能放在普通事务块里；
- 删除索引也可用 `DROP INDEX CONCURRENTLY`。

示例：

```sql
CREATE INDEX CONCURRENTLY submissions_user_updated_idx
ON submissions(user_id, updated_at DESC, id DESC);
```

开发环境可以直接建索引，生产大表要考虑并发建索引。

## 11. 复制、高可用和日志

### 11.1 MySQL

MySQL 常见关键词：

- redo log；
- undo log；
- binlog；
- relay log；
- GTID；
- 主从复制；
- 半同步复制；
- MGR。

binlog 是 MySQL 生态中非常重要的东西：

- 主从复制；
- point-in-time recovery；
- CDC；
- Canal/Debezium 等数据同步。

### 11.2 PostgreSQL

PostgreSQL 常见关键词：

- WAL；
- checkpoint；
- streaming replication；
- replication slot；
- logical replication；
- hot standby；
- PITR。

WAL 类似“预写日志”核心机制，既服务崩溃恢复，也服务复制和归档恢复。

### 11.3 后端开发要知道什么

如果你不是 DBA，至少要知道：

- 两者都不是“写入返回成功就绝对不会丢”，还取决于 fsync、复制、提交策略；
- 主从延迟会影响读写分离；
- 读从库可能读不到刚写入的数据；
- 事务性 outbox 依赖主库事务提交，不能把关键读写随便打到从库。

## 12. 运维心智差异

### 12.1 MySQL 常见关注点

- buffer pool 命中率；
- 慢查询；
- redo log 写入；
- undo log 和 history list；
- 主从延迟；
- 死锁；
- 大事务；
- online DDL；
- binlog 体积。

### 12.2 PostgreSQL 常见关注点

- autovacuum 是否及时；
- dead tuple；
- table/index bloat；
- WAL 量；
- checkpoint；
- replication slot 是否导致 WAL 堆积；
- idle in transaction；
- 统计信息是否准确；
- 连接数和连接池。

PostgreSQL 特别要警惕：

```text
idle in transaction
```

长时间打开事务会阻碍 vacuum 清理旧版本，导致表膨胀。

## 13. 常见需求如何从 MySQL 迁移到 PostgreSQL

### 13.1 用户表大小写唯一

MySQL 常见：

```sql
username VARCHAR(64) NOT NULL UNIQUE
```

如果 collation 是 case-insensitive，`Kai` 和 `kai` 冲突。

PostgreSQL 推荐显式：

```sql
username TEXT NOT NULL,
username_normalized TEXT NOT NULL UNIQUE
```

应用写入时：

```go
strings.ToLower(strings.TrimSpace(username))
```

好处：

- 跨数据库一致；
- 面试解释清楚；
- 不依赖隐式 collation。

### 13.2 分页

MySQL offset 分页：

```sql
SELECT *
FROM submissions
WHERE user_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 20 OFFSET 1000;
```

PostgreSQL 同样可以。

更推荐 keyset pagination：

```sql
SELECT *
FROM submissions
WHERE user_id = $1
  AND (created_at, id) < ($2, $3)
ORDER BY created_at DESC, id DESC
LIMIT 20;
```

MySQL 也可以写类似逻辑：

```sql
WHERE user_id = ?
  AND (created_at < ? OR (created_at = ? AND id < ?))
```

### 13.3 幂等插入

MySQL：

```sql
INSERT IGNORE INTO problem_tags(problem_id, tag)
VALUES (?, ?);
```

PostgreSQL：

```sql
INSERT INTO problem_tags(problem_id, tag)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;
```

### 13.4 任务抢占

MySQL 8：

```sql
START TRANSACTION;

SELECT id
FROM judge_outbox
WHERE published_at IS NULL
  AND next_attempt_at <= UTC_TIMESTAMP(6)
ORDER BY id
LIMIT 10
FOR UPDATE SKIP LOCKED;

UPDATE judge_outbox
SET claimed_by = ?, claim_expires_at = ?
WHERE id IN (...);

COMMIT;
```

PostgreSQL：

```sql
WITH candidates AS (
  SELECT id
  FROM judge_outbox
  WHERE published_at IS NULL
    AND next_attempt_at <= now()
  ORDER BY id
  FOR UPDATE SKIP LOCKED
  LIMIT $1
)
UPDATE judge_outbox AS outbox
SET claimed_by = $2,
    claim_expires_at = $3
FROM candidates
WHERE outbox.id = candidates.id
RETURNING outbox.id, outbox.submission_id;
```

PostgreSQL 写法更紧凑；MySQL 写法更强调事务步骤。

## 14. 在 Go 项目中的使用差异

### 14.1 驱动

PostgreSQL 常用：

```go
github.com/jackc/pgx/v5
github.com/jackc/pgx/v5/pgxpool
```

MySQL 常用：

```go
database/sql
github.com/go-sql-driver/mysql
```

pgx 的类型能力更贴近 PostgreSQL，比如数组、jsonb、copy、通知等。MySQL 驱动通常通过 `database/sql` 使用。

### 14.2 时间

建议无论 MySQL 还是 PostgreSQL：

```go
now := time.Now().UTC()
```

数据库：

PostgreSQL：

```sql
TIMESTAMPTZ NOT NULL
```

MySQL：

```sql
DATETIME(6) NOT NULL
```

然后约定应用层全部 UTC。

### 14.3 错误处理

PostgreSQL 唯一冲突通常看 SQLSTATE：

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return ErrConflict
}
```

MySQL 常见看错误码：

```go
var mysqlErr *mysql.MySQLError
if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
    return ErrConflict
}
```

## 15. GoJudge 项目里的 PostgreSQL 特性如何映射到 MySQL

| 当前 PostgreSQL 写法 | MySQL 8/InnoDB 等价思路 |
| --- | --- |
| `TIMESTAMPTZ` | `DATETIME(6)` + 应用层 UTC |
| `$1` 参数 | `?` 参数 |
| `BIGSERIAL` / identity | `BIGINT AUTO_INCREMENT` |
| `ON CONFLICT DO NOTHING` | `INSERT IGNORE` 或 `ON DUPLICATE KEY` |
| `ON CONFLICT DO UPDATE` | `ON DUPLICATE KEY UPDATE` |
| `UPDATE ... FROM ... RETURNING` | 事务内 `SELECT FOR UPDATE` + `UPDATE` + `SELECT` |
| partial index | 联合索引带状态列，或生成列 |
| `array_agg` | `JSON_ARRAYAGG`、`GROUP_CONCAT` 或拆查询 |
| `FOR UPDATE SKIP LOCKED` | MySQL 8 支持，但要重测隔离级别和 gap lock |
| `pgxpool` | `database/sql` + MySQL driver |

GoJudge 这种系统迁移到 MySQL 时，最需要重新验证：

1. Outbox 并发抢占是否会锁范围过大；
2. `SELECT FOR UPDATE SKIP LOCKED` 在默认 REPEATABLE READ 下是否符合预期；
3. 过期租约接管是否存在 gap lock 或死锁；
4. 条件更新 `WHERE token AND status AND lease_expires_at` 是否稳定；
5. 时间精度和时区是否一致；
6. 唯一冲突、幂等 seed、排行榜聚合 SQL 是否改写正确。

## 16. 学习路线：从 MySQL 迁移到 PostgreSQL

### 第一阶段：语法等价

你要能快速改写：

- `AUTO_INCREMENT` → identity/sequence；
- `?` → `$1`；
- `ON DUPLICATE KEY` → `ON CONFLICT`；
- `DATETIME` → `TIMESTAMPTZ`；
- `LIMIT offset, size` → `LIMIT size OFFSET offset`；
- `INSERT IGNORE` → `ON CONFLICT DO NOTHING`。

### 第二阶段：事务和锁

重点理解：

- PostgreSQL 默认 READ COMMITTED；
- `FOR UPDATE SKIP LOCKED`；
- `NOWAIT`；
- `UPDATE ... RETURNING`；
- deadlock 错误重试；
- 事务越短越好。

### 第三阶段：MVCC 和 vacuum

必须能解释：

- PostgreSQL 更新产生新 tuple；
- dead tuple 需要 vacuum；
- autovacuum 为什么重要；
- 长事务为什么危险；
- table bloat/index bloat 是什么。

### 第四阶段：索引和执行计划

重点掌握：

- `EXPLAIN (ANALYZE, BUFFERS)`；
- Seq Scan vs Index Scan vs Index Only Scan；
- partial index；
- expression index；
- GIN/jsonb；
- 统计信息和 `ANALYZE`。

### 第五阶段：工程迁移

拿 GoJudge 练习：

1. 把 `PostgresStore` 接口抽象成 `SQLStore`；
2. 写 `MySQLStore`；
3. 保留同一组 store contract tests；
4. 使用 MySQL 8 integration tests；
5. 对 Outbox/lease/fencing 做并发测试；
6. 再决定简历里是否能写 MySQL 实现。

## 17. 高频面试问答

### Q1：PostgreSQL 和 MySQL 最大区别是什么？

简答：

> MySQL/InnoDB 更强调聚簇索引、undo/redo 和 next-key lock；PostgreSQL 更强调 heap tuple MVCC、vacuum、强 SQL 表达和丰富索引类型。

### Q2：PostgreSQL 为什么需要 vacuum？

简答：

> PostgreSQL 更新和删除会留下旧 tuple 版本，等没有事务需要它们后，vacuum 负责清理 dead tuple，避免表和索引持续膨胀。

### Q3：MySQL 默认 REPEATABLE READ，PostgreSQL 默认 READ COMMITTED，有什么影响？

简答：

> MySQL 默认同一事务内快照读通常保持同一 Read View；PostgreSQL 默认每条语句一个新快照，所以同一事务内后续 SELECT 可能看到其他事务已提交的数据。

### Q4：PostgreSQL 的 `RETURNING` 有什么用？

简答：

> 可以在 INSERT/UPDATE/DELETE 后直接返回受影响行，减少一次查询，也方便把条件更新和结果读取放在同一条 SQL 中。

### Q5：PostgreSQL partial index 如何在 MySQL 实现？

简答：

> MySQL 没有直接等价 partial index，通常用联合索引带状态列，或者用生成列表达条件后再建索引。

### Q6：这个项目为什么用 PostgreSQL 而不是 MySQL？

简答：

> PostgreSQL 的 `SKIP LOCKED`、`UPDATE ... RETURNING`、partial index 和丰富 SQL 表达让 Outbox、租约、fencing 等可靠性逻辑写得更直接。MySQL 8 InnoDB 也能实现，但需要改写 SQL 并重新验证锁和隔离级别。

### Q7：如果面试官问你只会 MySQL，为什么项目用 PostgreSQL？

建议回答：

> 我熟悉 MySQL 的事务、索引和 InnoDB 锁模型。这个项目选择 PostgreSQL 是为了更直接地实现 Outbox、lease 和 fencing token，例如 `FOR UPDATE SKIP LOCKED`、`UPDATE ... RETURNING` 和 partial index。核心设计并不绑定 PostgreSQL，迁移到 MySQL 8 时会把这些 SQL 改成事务内多步操作，并重点验证默认 REPEATABLE READ、gap lock、时间类型和唯一冲突处理。

## 18. 不要混淆的说法

| 错误说法 | 更准确说法 |
| --- | --- |
| PostgreSQL 一定比 MySQL 强 | 两者侧重点不同，要看业务、团队和运维能力 |
| MySQL 没有 MVCC | InnoDB 有 MVCC，主要基于 undo log 和 Read View |
| PostgreSQL 更新就是原地修改 | PostgreSQL 更新通常产生新 tuple 版本 |
| PostgreSQL 不需要维护 | PostgreSQL 需要关注 vacuum、bloat、WAL 和长事务 |
| MySQL 不能做任务队列抢占 | MySQL 8 支持 `SKIP LOCKED`，但要验证锁行为 |
| PostgreSQL 的主键就是聚簇索引 | PostgreSQL 主键是唯一索引，表不是默认按主键聚簇 |
| 会 MySQL 就能直接替换 PostgreSQL | 需要改 SQL 方言、锁行为、时间类型、索引和测试 |

## 19. 对 GoJudge 的最终面试表述

可以这样说：

> 当前项目实际使用 PostgreSQL，因为它在事务化任务抢占、条件更新返回、部分索引和复杂聚合方面写法更直接。我的 MySQL 基础可以迁移这套设计：核心仍是事务、行锁、唯一约束、条件更新和 `SKIP LOCKED`。迁移时不能只换驱动，要把 `RETURNING`、`ON CONFLICT`、partial index、时间类型和默认隔离级别逐一替换并补 integration tests。

不要这样说：

```text
这个项目 PostgreSQL 和 MySQL 可以无成本平替。
```

更准确：

```text
架构模式可迁移，SQL 和锁语义需要重写和验证。
```
