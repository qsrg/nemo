-- 008_menu_seed.sql
-- 菜单/按钮 + 资源类型基线 seed（环境初始化用，非运行时事实源）。
--
-- 定位（2026-10-11，替代 deploy/menus.yaml + scripts/apply-config.py 下发路径）：
--   - 运行时事实源是 authz 的 menus 表（MySQL）：生产经管理面 CRUD
--     （菜单管理页 / POST /v1/admin/menus）维护，me/menus 从库读；
--   - 本文件是"新环境初始化基线"快照：authz cmd/provision 按文件名顺序
--     执行（schema_migrations 幂等，仅首次跑），新环境一键初始化脚本
--     scripts/init-env.sh 依赖它重建菜单/按钮/资源类型；
--   - 基线之后菜单改动经管理面操作，本文件不随日常改动更新；需要重做
--     基线时从开发库重新导出（mysqldump 见 scripts/init-env.sh 注释）。
--   - 资源类型随菜单/按钮声明派生（隐式注册）；本文件 seed 注册表行是为
--     新环境免启动链路二次派生（管理码 ManageCode/CreateCode 声明驱动）。
--
-- 幂等：INSERT IGNORE（主键/唯一键冲突时忽略，重复执行安全；配合
-- schema_migrations 双保险）。create_by/update_by 归一 'seed' 标识来源。

INSERT IGNORE INTO `menus` (`id`,`type`,`parent_id`,`title`,`icon`,`path`,`sort`,`permission_code`,`resource_type`,`relation`,`action_codes`,`enabled`,`create_by`,`update_by`,`created_at`,`updated_at`) VALUES
(1,'menu',NULL,'安全审计','Files','/audit',150,'audit:read','','','',1,'seed','seed',NOW(),NOW()),
(5,'menu',NULL,'RocketMQ','Connection','/rocketmq',40,'rocketmq:read','','','',1,'seed','seed',NOW(),NOW()),
(7,'menu',NULL,'NuJob','List','/nujob',100,NULL,'','','',1,'seed','seed',NOW(),NOW()),
(9,'menu',353,'资源共享','','/authz/shares',210,'authz-shares:read','','','',1,'seed','seed',NOW(),NOW()),
(11,'menu',NULL,'Redis','Coin','/redis',10,'redis:read','redis_instance,redis_cluster','','',1,'seed','seed',NOW(),NOW()),
(12,'menu',353,'人员授权','Lock','/authz/grants',170,'authz-grants:read','','','',1,'seed','seed',NOW(),NOW()),
(13,'menu',NULL,'Kafka','DataLine','/kafka',50,'kafka:read','','','',1,'seed','seed',NOW(),NOW()),
(14,'menu',353,'审计','','/authz/audit',200,'authz-audit:read','','','',1,'seed','seed',NOW(),NOW()),
(15,'menu',13,'Topic 管理','Connection','/kafka/topics',60,'kafka-topics:read','kafka_topic','','',1,'seed','seed',NOW(),NOW()),
(16,'menu',353,'菜单管理','','/authz/menus',190,'authz-menus:read','','','',1,'seed','seed',NOW(),NOW()),
(17,'menu',NULL,'Zookeeper','Connection','/zookeeper',30,'zookeeper:read','zk_cluster','','',1,'seed','seed',NOW(),NOW()),
(18,'menu',353,'部门与成员','','/authz/orgs',180,'authz-orgs:read','','','',1,'seed','seed',NOW(),NOW()),
(109,'menu',NULL,'智能问答','','/mwbot',60,'mwbot:read','','','',1,'seed','seed',NOW(),NOW()),
(132,'menu',353,'资源属性','','/authz/attributes',230,'authz-attributes:read','','','',1,'seed','seed',NOW(),NOW()),
(133,'menu',353,'ABAC 策略','','/authz/policies',240,'authz-policies:read','','','',1,'seed','seed',NOW(),NOW()),
(332,'button',15,'新建 Topic','','',1,'kafka-topics:create','','','',1,'seed','seed',NOW(),NOW()),
(333,'button',15,'导出','','',2,'kafka-topics:export','','','',1,'seed','seed',NOW(),NOW()),
(349,'button',15,'删除 Topic','','',4,'kafka-topics:delete','kafka_topic','can_delete','',1,'seed','seed',NOW(),NOW()),
(353,'menu',NULL,'权限管理','Lock','/authz',165,'authz:read','','','',1,'seed','seed',NOW(),NOW()),
(357,'menu',13,'集群管理','DataBoard','/kafka/clusters',70,'kafka-clusters:read','','','',1,'seed','seed',NOW(),NOW()),
(358,'button',15,'编辑 Topic','','',0,'kafka-topics:edit','kafka_topic','can_edit','',1,'seed','seed',NOW(),NOW()),
(360,'button',15,'配置权限','','',6,'kafka-topics:manage-grants','kafka_topic','can_manage_grants','',1,'seed','seed',NOW(),NOW()),
(361,'button',15,'查看消费者','','',0,'kafka-topics:view-consumers','kafka_topic','can_view','',1,'seed','seed',NOW(),NOW()),
(362,'button',15,'重置位点','','',8,'kafka-topics:reset-offset','kafka_topic','can_edit','',1,'seed','seed',NOW(),NOW()),
(370,'button',11,'新建实例','','',1,'redis:create','','','',1,'seed','seed',NOW(),NOW()),
(371,'button',11,'编辑实例','','',2,'redis:edit','redis_instance','can_edit','',1,'seed','seed',NOW(),NOW()),
(372,'button',11,'删除实例','','',3,'redis:delete','redis_instance','can_delete','',1,'seed','seed',NOW(),NOW()),
(373,'button',15,'冻结 Topic','','',3,'kafka-topics:freeze','kafka_topic','can_edit','',1,'seed','seed',NOW(),NOW()),
(378,'menu',11,'实例管理','','/redis/instances',10,NULL,'','','',1,'seed','seed',NOW(),NOW()),
(380,'menu',17,'节点浏览','','/zookeeper/nodes',10,NULL,'','','',1,'seed','seed',NOW(),NOW()),
(381,'menu',5,'RMQ Topic 管理','','/rocketmq/topics',10,'rocketmq-topics:read','rmq_topic','','',1,'seed','seed',NOW(),NOW()),
(385,'menu',7,'作业列表','','/nujob/jobs',10,'nujob:read','','','',1,'seed','seed',NOW(),NOW()),
(390,'menu',1,'审计日志','','/audit/logs',10,NULL,'','','',1,'seed','seed',NOW(),NOW()),
(392,'menu',109,'智能问答','','/mwbot/chat',10,NULL,'','','',1,'seed','seed',NOW(),NOW()),
(393,'button',381,'新建 Topic','','',1,'rocketmq-topics:create','','','',1,'seed','seed',NOW(),NOW()),
(394,'button',381,'删除 Topic','','',2,'rocketmq-topics:delete','rmq_topic','can_delete','',1,'seed','seed',NOW(),NOW()),
(395,'button',381,'编辑 Topic','','',1,'rocketmq-topics:edit','rmq_topic','can_edit','',1,'seed','seed',NOW(),NOW()),
(405,'menu',NULL,'存储池','Box','/storage',20,'storage:read','','','',1,'seed','seed',NOW(),NOW()),
(406,'menu',NULL,'事件查询','Share','/eventbus',70,'eventbus:read','','','',1,'seed','seed',NOW(),NOW()),
(407,'menu',NULL,'订阅管理','DataAnalysis','/cds',80,'cds:read','','','',1,'seed','seed',NOW(),NOW()),
(408,'menu',NULL,'调度任务','Odometer','/schedule',90,'schedule:read','','','',1,'seed','seed',NOW(),NOW()),
(409,'menu',13,'集群概览','DataLine','/kafka/overview',55,NULL,'','','',1,'seed','seed',NOW(),NOW()),
(410,'menu',NULL,'集群监控','Monitor','/monitor',110,'monitor:read','','','',1,'seed','seed',NOW(),NOW()),
(411,'menu',NULL,'组件值班','Service','/oncall',120,'oncall:read','','','',1,'seed','seed',NOW(),NOW()),
(412,'menu',NULL,'组件安装','Upload','/install',130,'install:read','','','',1,'seed','seed',NOW(),NOW()),
(413,'menu',NULL,'配置核查','Stamp','/configcheck',140,'configcheck:read','','','',1,'seed','seed',NOW(),NOW()),
(414,'menu',NULL,'双人复核','User','/review',160,'review:read','','','',1,'seed','seed',NOW(),NOW());

INSERT IGNORE INTO `resource_types` (`id`,`name`,`dsl_block`,`manage_code`,`create_code`,`create_by`,`update_by`,`created_at`,`updated_at`) VALUES
(2,'kafka_topic','type kafka_topic\n  relations\n    define org: [org]\n    define owner: [user, org#member]\n    define editor: [user, org#member] or owner\n    define viewer: [user, org#member] or editor\n    define member_view: viewer or member from org\n    define can_view: member_view or owner\n    define can_edit: editor\n    define can_delete: owner\n    define can_manage_grants: owner','kafka-topics:manage','kafka-topics:create','seed','seed',NOW(),NOW()),
(6,'rmq_topic','type rmq_topic\n  relations\n    define org: [org]\n    define owner: [user, org#member]\n    define editor: [user, org#member] or owner\n    define viewer: [user, org#member] or editor\n    define member_view: viewer or member from org\n    define can_view: member_view or owner\n    define can_edit: editor\n    define can_delete: owner','rocketmq-topics:manage','rocketmq-topics:create','seed','seed',NOW(),NOW()),
(7,'redis_instance','type redis_instance\n  relations\n    define org: [org]\n    define owner: [user, org#member]\n    define editor: [user, org#member] or owner\n    define viewer: [user, org#member] or editor\n    define member_view: viewer or member from org\n    define can_view: member_view or owner\n    define can_edit: editor\n    define can_delete: owner','redis:manage','redis:create','seed','seed',NOW(),NOW()),
(8,'redis_cluster','type redis_cluster\n  relations\n    define org: [org]\n    define owner: [user, org#member]\n    define editor: [user, org#member] or owner\n    define viewer: [user, org#member] or editor\n    define member_view: viewer or member from org\n    define can_view: member_view or owner','redis:manage','','seed','seed',NOW(),NOW()),
(9,'zk_cluster','type zk_cluster\n  relations\n    define org: [org]\n    define owner: [user, org#member]\n    define editor: [user, org#member] or owner\n    define viewer: [user, org#member] or editor\n    define member_view: viewer or member from org\n    define can_view: member_view or owner','zookeeper:manage','','seed','seed',NOW(),NOW()),
(10,'kafka_cluster','type kafka_cluster\n  relations\n    define org: [org]\n    define owner: [user, org#member]\n    define editor: [user, org#member] or owner\n    define viewer: [user, org#member] or editor\n    define member_view: viewer or member from org\n    define can_view: member_view or owner','','','seed','seed',NOW(),NOW());
