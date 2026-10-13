CREATE ROLE cj_platform LOGIN PASSWORD 'cj-platform-local-only';
CREATE ROLE cj_judge LOGIN PASSWORD 'cj-judge-local-only';
CREATE DATABASE cj_platform OWNER cj_platform;
CREATE DATABASE cj_judge OWNER cj_judge;
REVOKE CONNECT ON DATABASE cj_platform FROM PUBLIC;
REVOKE CONNECT ON DATABASE cj_judge FROM PUBLIC;
GRANT CONNECT ON DATABASE cj_platform TO cj_platform;
GRANT CONNECT ON DATABASE cj_judge TO cj_judge;

