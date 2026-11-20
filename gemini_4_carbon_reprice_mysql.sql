-- 一次性运维脚本；不创建备份表，不连接任何数据库。
-- MySQL 5.7.8+ / 8.x，InnoDB，主库和日志同库；在已选中目标数据库的新会话运行。
-- 调用账号需要 CREATE ROUTINE / EXECUTE、临时表及相应表的 SELECT / UPDATE 权限。
-- 时间：北京时间 2026-11-11 00:00:00 <= created_at < 2026-11-14 21:33:18。
-- 输入 / 输出 / 缓存读取：2 / 20 / 0.1 -> 4 / 20 / 0.2 USD / 百万 tokens。
-- 只改 logs、quota_data 和现存 users/tokens/channels 的累计 used_quota。
-- 不改 users.quota、tokens.remain_quota、套餐额度、token 数量或请求次数。
-- 不修改未来请求的定价配置。先在管理界面改好新价格。
--
-- 适用范围：旧倍率，或单档 tier("标签", p*2+c*20+cr*0.1) 动态定价。
-- 自定义阶梯/请求倍率/工具附加费/独立音频计费/缓存写入等不能盲算，遇到会停止当前批次。
-- 验证历史金额能按旧公式精确复算；不能复算则回滚当前批次，输出日志 ID。
-- 图表行必须唯一匹配；未记录节点且同一统计维度有多个节点时停止，不分摊猜测。
-- 要求 logs 存在当前仓库的 idx_created_at_id(created_at,id) 索引，quota_data 为当前表结构。
-- 历史 QuotaPerUnit 应未改变；读取 options 当前值，未配置时采用项目默认 500000。
-- 假设这段历史没有未记录的附加倍率或人工改账；日志不足以还原任意自定义计费。
--
-- 大数据执行：每批只扫描 p_batch_size 条日志，采用 (created_at,id) 游标，不使用 OFFSET。
-- 每批独立提交。发生错误只回滚当前批次；之前已提交的批次保留。
-- 原值仅存在于会话临时计算表；连接关闭即消失，不能用它们撤销已提交批次。
-- 每条修正日志增加 reprice_gc_20261114_v1 标记，重跑不会重复累加。
-- 运行前确认截止时间之前的异步日志、累计额度和图表缓存均已落库。
-- 避免同时进行日志清理、统计重建和其他历史修正；留意锁等待、I/O 和复制延迟。
-- tokens.used_quota 的 Redis 展示缓存需要通过现有失效机制刷新，不能执行 FLUSHALL。
-- 本脚本尚未在真实 MySQL 或生产数据上执行验证。
--
-- 导入本文件只安装存储过程；以下 CALL 才修改数据。
-- 首次执行：CALL reprice_gc_20261114(0, 0, 0, 500, 20);
-- 同一会话续跑：
-- CALL reprice_gc_20261114(@gc_cursor_ts, @gc_cursor_id, @gc_upper_id, 500, 20);
-- 换会话：把最后一次 committed 输出的 cursor_ts/cursor_id/upper_id 填回 CALL。
-- 扫描到末尾会输出 finished=1。没有进度可从 0 重跑，但会重新扫描之前的范围。
-- 完成后可删除过程：DROP PROCEDURE reprice_gc_20261114;

SET NAMES utf8mb4;
SET SESSION time_zone = '+08:00';

DELIMITER $$
CREATE PROCEDURE reprice_gc_20261114(
    IN p_cursor_ts BIGINT,
    IN p_cursor_id BIGINT,
    IN p_upper_id BIGINT,
    IN p_batch_size INT,
    IN p_max_batches INT
)
SQL SECURITY INVOKER
proc: BEGIN
    DECLARE v_start BIGINT DEFAULT 1794326400;
    DECLARE v_end BIGINT DEFAULT 1794663198;
    DECLARE v_ts BIGINT;
    DECLARE v_id BIGINT;
    DECLARE v_upper BIGINT;
    DECLARE v_next_ts BIGINT;
    DECLARE v_next_id BIGINT;
    DECLARE v_batch INT DEFAULT 0;
    DECLARE v_scanned INT DEFAULT 0;
    DECLARE v_changed INT DEFAULT 0;
    DECLARE v_bad INT DEFAULT 0;
    DECLARE v_finished INT DEFAULT 0;
    DECLARE v_unit_text VARCHAR(100);
    DECLARE v_unit DECIMAL(38,18);
    DECLARE v_delta DECIMAL(38,0) DEFAULT 0;
    DECLARE v_lock_name VARCHAR(64);
    DECLARE v_locked INT DEFAULT 0;
    DECLARE v_release INT;

    DECLARE EXIT HANDLER FOR SQLEXCEPTION, SQLWARNING
    BEGIN
        ROLLBACK;
        SET @gc_cursor_ts = v_ts, @gc_cursor_id = v_id, @gc_upper_id = v_upper;
        DROP TEMPORARY TABLE IF EXISTS gc_fix_page, gc_fix_work, gc_fix_match, gc_fix_delta;
        IF v_locked = 1 THEN
            SELECT RELEASE_LOCK(v_lock_name) INTO v_release;
        END IF;
        SELECT 'stopped_current_batch_rolled_back' AS status,
               v_ts AS cursor_ts, v_id AS cursor_id, v_upper AS upper_id,
               v_batch AS committed_batches;
        RESIGNAL;
    END;

    IF DATABASE() IS NULL OR p_batch_size IS NULL OR p_batch_size NOT BETWEEN 1 AND 2000
       OR p_max_batches IS NULL OR p_max_batches NOT BETWEEN 1 AND 10000
       OR p_cursor_ts IS NULL OR p_cursor_id IS NULL OR p_upper_id IS NULL
       OR p_cursor_id < 0 OR p_upper_id < 0 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Select a database; use valid cursor and batch parameters';
    END IF;
    SET v_ts = IF(p_cursor_ts = 0, v_start, p_cursor_ts);
    SET v_id = p_cursor_id;
    IF v_ts < v_start OR v_ts >= v_end THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Cursor timestamp is outside the repair interval';
    END IF;
    SET v_lock_name = CONCAT('gc-reprice-20261114-', MD5(DATABASE()));
    SELECT GET_LOCK(v_lock_name, 0) INTO v_locked;
    IF COALESCE(v_locked, 0) <> 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Another repair session is running';
    END IF;

    SELECT COUNT(*) INTO v_bad
    FROM information_schema.tables
    WHERE table_schema = DATABASE()
      AND table_name IN ('logs', 'quota_data', 'users', 'tokens', 'channels')
      AND engine = 'InnoDB';
    IF v_bad <> 5 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'All five target tables must exist and use InnoDB';
    END IF;
    SELECT COUNT(*) INTO v_bad
    FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'logs'
      AND index_name = 'idx_created_at_id'
      AND ((seq_in_index = 1 AND column_name = 'created_at')
        OR (seq_in_index = 2 AND column_name = 'id'));
    IF v_bad <> 2 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Required logs index idx_created_at_id(created_at,id) is missing';
    END IF;
    SELECT COALESCE(MAX(`value`), '500000') INTO v_unit_text
    FROM options WHERE `key` = 'QuotaPerUnit';
    IF v_unit_text NOT REGEXP '^[0-9]+([.][0-9]+)?$' THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Invalid QuotaPerUnit';
    END IF;
    SET v_unit = CAST(v_unit_text AS DECIMAL(38,18));
    IF v_unit <= 0 OR v_unit > 1000000000 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'QuotaPerUnit is outside the supported range';
    END IF;
    IF p_upper_id = 0 THEN
        SELECT COALESCE(MAX(id), 0) INTO v_upper FROM logs;
    ELSE
        SET v_upper = p_upper_id;
    END IF;
    SET @gc_cursor_ts = v_ts, @gc_cursor_id = v_id, @gc_upper_id = v_upper;

    CREATE TEMPORARY TABLE gc_fix_page (
        id BIGINT PRIMARY KEY, created_at BIGINT NOT NULL
    ) ENGINE=InnoDB;
    CREATE TEMPORARY TABLE gc_fix_work (
        id BIGINT PRIMARY KEY, user_id BIGINT, token_id BIGINT, channel_id BIGINT,
        username VARCHAR(255), model_name VARCHAR(255), use_group VARCHAR(255),
        hour_at BIGINT, prompt_tokens BIGINT, completion_tokens BIGINT,
        old_quota BIGINT, j JSON, valid_json INT,
        billing_mode VARCHAR(32), expr_raw LONGTEXT, expr_norm LONGTEXT,
        price_state VARCHAR(16) DEFAULT 'unknown', reason VARCHAR(120) DEFAULT '',
        p DECIMAL(24,0), cr DECIMAL(24,0), gr DECIMAL(38,18),
        old_raw DOUBLE, new_raw DOUBLE,
        old_calc DECIMAL(38,0), new_calc DECIMAL(38,0), delta DECIMAL(38,0), qid BIGINT
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
    CREATE TEMPORARY TABLE gc_fix_match (
        id BIGINT PRIMARY KEY, qid BIGINT, matches INT
    ) ENGINE=InnoDB;
    CREATE TEMPORARY TABLE gc_fix_delta (
        kind CHAR(1), id BIGINT, delta DECIMAL(38,0), PRIMARY KEY(kind,id)
    ) ENGINE=InnoDB;

    batch_loop: WHILE v_batch < p_max_batches DO
        -- 让 INSERT...SELECT 的分页读取不对整个扫描区间加共享 next-key 锁。
        -- 不更改全局隔离级别或 binlog 设置；环境不允许时本批报错退出。
        SET TRANSACTION ISOLATION LEVEL READ COMMITTED;
        START TRANSACTION;
        DELETE FROM gc_fix_page;
        DELETE FROM gc_fix_work;
        DELETE FROM gc_fix_match;
        DELETE FROM gc_fix_delta;

        -- 只分页扫描索引。先取本批 ID，再按主键锁定需要修正的目标模型日志。
        INSERT INTO gc_fix_page (id, created_at)
        SELECT id, created_at FROM logs FORCE INDEX (idx_created_at_id)
        WHERE created_at >= v_ts AND created_at < v_end
          AND (created_at > v_ts OR id > v_id) AND id <= v_upper
        ORDER BY created_at, id LIMIT p_batch_size;
        SET v_scanned = ROW_COUNT();
        IF v_scanned = 0 THEN
            COMMIT;
            SET v_finished = 1;
            LEAVE batch_loop;
        END IF;
        SELECT created_at, id INTO v_next_ts, v_next_id
        FROM gc_fix_page ORDER BY created_at DESC, id DESC LIMIT 1;

        INSERT INTO gc_fix_work
            (id,user_id,token_id,channel_id,username,model_name,use_group,hour_at,
             prompt_tokens,completion_tokens,old_quota,j,valid_json)
        SELECT l.id,l.user_id,l.token_id,l.channel_id,l.username,l.model_name,l.`group`,
               l.created_at - MOD(l.created_at,3600),l.prompt_tokens,l.completion_tokens,
               l.quota,IF(JSON_VALID(l.other),l.other,'{}'),COALESCE(JSON_VALID(l.other),0)
        FROM gc_fix_page s STRAIGHT_JOIN logs l ON l.id = s.id
        WHERE BINARY l.model_name = BINARY 'gemini-4-carbon' AND l.type IN (2,8)
          AND l.created_at >= v_start AND l.created_at < v_end
          AND JSON_CONTAINS_PATH(IF(JSON_VALID(l.other),l.other,'{}'),
                                'one','$.reprice_gc_20261114_v1') = 0
        FOR UPDATE;

        UPDATE gc_fix_work SET
            billing_mode = COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(j,'$.billing_mode')),'null'),'ratio'),
            expr_raw = CONVERT(FROM_BASE64(JSON_UNQUOTE(JSON_EXTRACT(j,'$.expr_b64'))) USING utf8mb4);
        UPDATE gc_fix_work SET expr_norm = REPLACE(REPLACE(REPLACE(REPLACE(
            expr_raw,' ',''),CHAR(9),''),CHAR(10),''),CHAR(13),'');
        UPDATE gc_fix_work SET price_state = CASE
            WHEN billing_mode = 'tiered_expr' AND expr_norm REGEXP
              '^(v1:)?tier[(]"[A-Za-z0-9_-]+",p[*]2([.]0+)?[+]c[*]20([.]0+)?[+]cr[*]0[.]10*[)]$' THEN 'old'
            WHEN billing_mode = 'tiered_expr' AND expr_norm REGEXP
              '^(v1:)?tier[(]"[A-Za-z0-9_-]+",p[*]4([.]0+)?[+]c[*]20([.]0+)?[+]cr[*]0[.]20*[)]$' THEN 'new'
            WHEN billing_mode IN ('ratio','')
              AND CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.model_ratio')) AS DECIMAL(38,18)) = 2*v_unit/1000000
              AND CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.completion_ratio')) AS DECIMAL(38,18)) = 10
              AND CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.cache_ratio')) AS DECIMAL(38,18)) = 0.05 THEN 'old'
            WHEN billing_mode IN ('ratio','')
              AND CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.model_ratio')) AS DECIMAL(38,18)) = 4*v_unit/1000000
              AND CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.completion_ratio')) AS DECIMAL(38,18)) = 5
              AND CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.cache_ratio')) AS DECIMAL(38,18)) = 0.05 THEN 'new'
            ELSE 'unknown' END;
        -- 已经是新价的记录不重复改，也不改它的统计。
        DELETE FROM gc_fix_work WHERE price_state = 'new';

        UPDATE gc_fix_work SET reason = CASE
            WHEN valid_json <> 1 OR JSON_TYPE(j) <> 'OBJECT' THEN 'invalid log metadata'
            WHEN price_state <> 'old' THEN 'unrecognized price snapshot; requires separate calculation'
            WHEN prompt_tokens IS NULL OR completion_tokens IS NULL OR old_quota IS NULL
              OR prompt_tokens < 0 OR completion_tokens < 0 OR old_quota < 0 THEN 'invalid tokens or quota'
            WHEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j,'$.cache_tokens')),'') NOT REGEXP '^[0-9]+$'
              OR COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j,'$.group_ratio')),'') NOT REGEXP '^[0-9]+([.][0-9]+)?$'
              THEN 'missing or invalid cache_tokens/group_ratio'
            WHEN JSON_UNQUOTE(JSON_EXTRACT(j,'$.claude')) = 'true'
              AND JSON_EXTRACT(j,'$.usage_semantic') IS NULL THEN 'ambiguous Claude cache semantics'
            WHEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j,'$.usage_semantic')),'openai') NOT IN ('openai','anthropic')
              THEN 'unknown usage semantics'
            WHEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(j,'$.billing_unit')),'token') <> 'token'
              OR JSON_CONTAINS_PATH(j,'one','$.fixed_price','$.usage_facts','$.ws','$.audio_input_seperate_price') = 1
              THEN 'nonstandard billing path'
            WHEN COALESCE(JSON_LENGTH(JSON_EXTRACT(j,'$.request_rules')),0) > 0
              OR COALESCE(JSON_LENGTH(JSON_EXTRACT(j,'$.tool_surcharges')),0) > 0
              OR JSON_CONTAINS_PATH(j,'one','$.admin_info.quota_saturation') = 1 THEN 'additional rules or surcharge'
            WHEN COALESCE(JSON_EXTRACT(j,'$.cache_creation_tokens')+0,0) <> 0
              OR COALESCE(JSON_EXTRACT(j,'$.cache_write_tokens')+0,0) <> 0
              OR COALESCE(JSON_EXTRACT(j,'$.cache_creation_tokens_5m')+0,0) <> 0
              OR COALESCE(JSON_EXTRACT(j,'$.cache_creation_tokens_1h')+0,0) <> 0
              OR COALESCE(JSON_EXTRACT(j,'$.cache_creation_tokens_12h')+0,0) <> 0 THEN 'cache write tokens present'
            WHEN billing_mode <> 'tiered_expr'
              AND (COALESCE(JSON_EXTRACT(j,'$.image_output')+0,0) <> 0
                OR COALESCE(JSON_EXTRACT(j,'$.audio_input_token_count')+0,0) <> 0
                OR COALESCE(JSON_EXTRACT(j,'$.model_price')+0,0) > 0) THEN 'separate legacy price component'
            ELSE '' END;
        SELECT COUNT(*) INTO v_bad FROM gc_fix_work WHERE reason <> '';
        IF v_bad > 0 THEN
            SELECT id, reason FROM gc_fix_work WHERE reason <> '' ORDER BY id LIMIT 20;
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Unsupported log pricing; current batch was not changed';
        END IF;

        UPDATE gc_fix_work SET
            cr = CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.cache_tokens')) AS DECIMAL(24,0)),
            gr = CAST(JSON_UNQUOTE(JSON_EXTRACT(j,'$.group_ratio')) AS DECIMAL(38,18));
        UPDATE gc_fix_work SET p = CASE
            WHEN JSON_UNQUOTE(JSON_EXTRACT(j,'$.usage_semantic')) = 'anthropic' THEN prompt_tokens
            ELSE prompt_tokens - cr END;
        SELECT COUNT(*) INTO v_bad FROM gc_fix_work
        WHERE p < 0 OR cr < 0 OR gr < 0 OR gr > 1000000000
           OR prompt_tokens > 2147483647 OR completion_tokens > 2147483647 OR cr > 2147483647;
        IF v_bad > 0 THEN
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Token counts or group ratio are outside the supported range';
        END IF;
        -- 先分别舍入新旧费用，再求差；不能对总费用或舍入后的差额简单乘二。
        UPDATE gc_fix_work SET
            old_calc = ROUND((p*2 + completion_tokens*20 + cr*0.1)*v_unit/1000000*gr,0),
            new_calc = ROUND((p*4 + completion_tokens*20 + cr*0.2)*v_unit/1000000*gr,0);
        -- 动态表达式使用 float64，保持 p、c、cr 的原运算顺序。
        -- MySQL ROUND(DOUBLE) 可能采用不同的半数舍入规则，显式模拟非负数的 math.Round。
        UPDATE gc_fix_work SET
            old_raw = (p*2e0 + completion_tokens*20e0 + cr*0.1e0)/1e6*(v_unit+0e0)*(gr+0e0),
            new_raw = (p*4e0 + completion_tokens*20e0 + cr*0.2e0)/1e6*(v_unit+0e0)*(gr+0e0)
        WHERE billing_mode='tiered_expr';
        UPDATE gc_fix_work SET
            old_calc = FLOOR(old_raw) + IF(old_raw-FLOOR(old_raw)>=0.5,1,0),
            new_calc = FLOOR(new_raw) + IF(new_raw-FLOOR(new_raw)>=0.5,1,0)
        WHERE billing_mode='tiered_expr';
        UPDATE gc_fix_work SET old_calc = GREATEST(old_calc,1), new_calc = GREATEST(new_calc,1)
        WHERE billing_mode <> 'tiered_expr' AND gr > 0 AND p+cr+completion_tokens > 0;
        UPDATE gc_fix_work SET delta = new_calc-old_quota;
        SELECT COUNT(*) INTO v_bad FROM gc_fix_work
        WHERE old_calc <> old_quota OR delta < 0 OR new_calc > 2147483647;
        IF v_bad > 0 THEN
            SELECT id,old_quota,old_calc,new_calc,delta FROM gc_fix_work
            WHERE old_calc <> old_quota OR delta < 0 OR new_calc > 2147483647 ORDER BY id LIMIT 20;
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Historical amount cannot be reproduced exactly; current batch rolled back';
        END IF;

        -- 保留所有统计维度。日志没写节点时，只接受唯一候选统计行。
        INSERT INTO gc_fix_match (id,qid,matches)
        SELECT b.id, MIN(q.id), COUNT(q.id)
        FROM gc_fix_work b LEFT JOIN quota_data q
          ON q.user_id = b.user_id AND BINARY q.username = BINARY b.username
         AND BINARY q.model_name = BINARY b.model_name AND q.created_at = b.hour_at
         AND BINARY q.use_group = BINARY b.use_group
         AND q.token_id = b.token_id AND q.channel_id = b.channel_id
         AND (JSON_EXTRACT(b.j,'$.admin_info.node_name') IS NULL
              OR BINARY q.node_name = BINARY JSON_UNQUOTE(JSON_EXTRACT(b.j,'$.admin_info.node_name')))
        WHERE b.delta <> 0 GROUP BY b.id;
        SELECT COUNT(*) INTO v_bad FROM gc_fix_match WHERE matches <> 1;
        IF v_bad > 0 THEN
            SELECT id,matches FROM gc_fix_match WHERE matches <> 1 ORDER BY id LIMIT 20;
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Missing or ambiguous quota_data row; current batch rolled back';
        END IF;
        UPDATE gc_fix_work b JOIN gc_fix_match m ON m.id=b.id SET b.qid=m.qid;
        INSERT INTO gc_fix_delta SELECT 'q',qid,SUM(delta) FROM gc_fix_work WHERE delta<>0 GROUP BY qid;
        INSERT INTO gc_fix_delta SELECT 'u',user_id,SUM(delta) FROM gc_fix_work WHERE delta<>0 AND user_id>0 GROUP BY user_id;
        INSERT INTO gc_fix_delta SELECT 't',token_id,SUM(delta) FROM gc_fix_work WHERE delta<>0 AND token_id>0 GROUP BY token_id;
        INSERT INTO gc_fix_delta SELECT 'c',channel_id,SUM(delta) FROM gc_fix_work WHERE delta<>0 AND channel_id>0 GROUP BY channel_id;

        UPDATE gc_fix_work b STRAIGHT_JOIN logs l ON l.id=b.id
        SET l.quota=b.new_calc,
            l.other=JSON_SET(
                CASE WHEN b.billing_mode='tiered_expr' THEN
                    JSON_SET(b.j,'$.expr_b64',REPLACE(TO_BASE64(CONCAT(
                        SUBSTRING_INDEX(b.expr_raw,',',1),', p * 4 + c * 20 + cr * 0.2)')),CHAR(10),''))
                ELSE JSON_SET(b.j,'$.model_ratio',4*v_unit/1000000,'$.completion_ratio',5,'$.cache_ratio',0.05) END,
                '$.reprice_gc_20261114_v1',JSON_OBJECT('at',UNIX_TIMESTAMP(),'wallet_debited',JSON_EXTRACT('false','$')));
        SET v_changed=ROW_COUNT();
        SELECT COUNT(*) INTO v_bad FROM gc_fix_work;
        IF v_changed <> v_bad THEN
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Log update count mismatch';
        END IF;

        UPDATE gc_fix_delta d STRAIGHT_JOIN quota_data q ON q.id=d.id
        SET q.quota=q.quota+d.delta WHERE d.kind='q';
        SET v_bad=ROW_COUNT();
        IF v_bad <> (SELECT COUNT(*) FROM gc_fix_delta WHERE kind='q') THEN
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Statistics update count mismatch';
        END IF;
        UPDATE gc_fix_delta d STRAIGHT_JOIN users u ON u.id=d.id
        SET u.used_quota=u.used_quota+d.delta WHERE d.kind='u';
        UPDATE gc_fix_delta d STRAIGHT_JOIN tokens t ON t.id=d.id
        SET t.used_quota=t.used_quota+d.delta WHERE d.kind='t';
        UPDATE gc_fix_delta d STRAIGHT_JOIN channels c ON c.id=d.id
        SET c.used_quota=c.used_quota+d.delta WHERE d.kind='c';
        SELECT COALESCE(SUM(delta),0) INTO v_delta FROM gc_fix_work;

        COMMIT;
        SET v_ts=v_next_ts, v_id=v_next_id, v_batch=v_batch+1;
        SET @gc_cursor_ts=v_ts, @gc_cursor_id=v_id, @gc_upper_id=v_upper;
        SELECT 'committed' AS status,v_batch AS batch_no,v_scanned AS scanned_rows,
               v_changed AS corrected_logs,v_delta AS added_quota,
               v_delta/v_unit AS added_statistical_usd,
               v_ts AS cursor_ts,v_id AS cursor_id,v_upper AS upper_id;
        DO SLEEP(0.1);
    END WHILE;

    DROP TEMPORARY TABLE gc_fix_page,gc_fix_work,gc_fix_match,gc_fix_delta;
    SELECT RELEASE_LOCK(v_lock_name) INTO v_release;
    SET v_locked=0;
    SELECT v_finished AS finished,v_batch AS committed_batches,
           @gc_cursor_ts AS cursor_ts,@gc_cursor_id AS cursor_id,@gc_upper_id AS upper_id;
END$$
DELIMITER ;
