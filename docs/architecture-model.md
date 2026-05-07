# LingEchoX 模型架构图

## 整体架构概览

```mermaid
graph TB
    subgraph "基础层"
        BM[BaseModel<br/>- ID, CreatedAt, UpdatedAt<br/>- IsDeleted, CreateBy, UpdateBy]
    end

    subgraph "多租户 RBAC 层"
        T[Tenant<br/>租户]
        TU[TenantUser<br/>租户用户]
        TG[TenantGroup<br/>租户组]
        TUG[TenantUserGroup<br/>用户组关联]
        TR[TenantRole<br/>租户角色]
        TUR[TenantUserRole<br/>用户角色关联]
        P[Permission<br/>权限]
        TRP[TenantRolePermission<br/>角色权限关联]
        PA[PlatformAdmin<br/>平台管理员]
    end

    subgraph "SIP 外呼活动层"
        SC[SIPCampaign<br/>外呼活动]
        SCC[SIPCampaignContact<br/>活动联系人]
        SCA[SIPCallAttempt<br/>呼叫尝试]
        SSR[SIPScriptRun<br/>脚本执行记录]
        SCE[SIPCampaignEvent<br/>活动事件]
    end

    subgraph "ACD 路由层"
        ACD[ACDPoolTarget<br/>ACD池目标]
        ASS[ACDShiftSchedule<br/>班次调度]
    end

    subgraph "资源管理层"
        TRK[Trunk<br/>线路]
        TRKN[TrunkNumber<br/>线路号码]
        SST[SIPScriptTemplate<br/>脚本模板]
        CR[Credential<br/>API凭证]
    end

    %% 继承关系
    BM --> T
    BM --> TU
    BM --> TG
    BM --> TUG
    BM --> TR
    BM --> TUR
    BM --> P
    BM --> TRP
    BM --> PA
    BM --> SC
    BM --> SCC
    BM --> SCA
    BM --> SSR
    BM --> SCE
    BM --> ACD
    BM --> ASS
    BM --> SST
    BM --> CR

    %% 关联关系
    T --> TU
    T --> TG
    T --> TR
    T --> SC
    T --> ACD
    T --> SST
    T --> TRK
    T --> CR

    TU --> TUG
    TG --> TUG
    TU --> TUR
    TR --> TUR
    TR --> TRP
    P --> TRP

    SC --> SCC
    SC --> SCA
    SC --> SSR
    SC --> SCE
    SCC --> SCA
    SCC --> SSR
    SCC --> SCE
    SCA --> SSR
    SCA --> SCE

    TRK --> TRKN
    ACD --> TRK
```

## ER 实体关系图

```mermaid
erDiagram
    %% 基础与多租户
    BASEMODEL {
        uint ID PK
        time CreatedAt
        time UpdatedAt
        int8 IsDeleted
        string CreateBy
        string UpdateBy
    }

    TENANT {
        uint ID PK
        string Name
        string Slug UK
        string Description
        string Status
    }

    TENANT_USER {
        uint ID PK
        uint TenantID FK
        string Email UK
        string Phone
        string Username
        string PasswordHash
        string DisplayName
        string Status
        time LastLoginAt
    }

    TENANT_GROUP {
        uint ID PK
        uint TenantID FK
        string Name
        bool IsDefault
    }

    TENANT_USER_GROUP {
        uint ID PK
        uint TenantUserID FK
        uint GroupID FK
    }

    TENANT_ROLE {
        uint ID PK
        uint TenantID FK
        string Name
        string Description
        bool IsSystem
    }

    TENANT_USER_ROLE {
        uint ID PK
        uint TenantUserID FK
        uint RoleID FK
    }

    PERMISSION {
        uint ID PK
        string Code UK
        string Name
        string Description
        string Resource
        string Action
    }

    TENANT_ROLE_PERMISSION {
        uint ID PK
        uint RoleID FK
        uint PermissionID FK
    }

    PLATFORM_ADMIN {
        uint ID PK
        string Email UK
        string PasswordHash
        string DisplayName
        string Status
    }

    %% SIP 外呼活动
    SIP_CAMPAIGN {
        uint ID PK
        uint TenantID FK
        string Name
        string Status
        string Scenario
        string MediaProfile
        string ScriptID
        string ScriptVersion
        json ScriptSpec
        string SystemPrompt
        string OpeningMessage
        string ClosingMessage
        string RetrySchedule
        int MaxAttempts
        int MaxCallDuration
        string OutboundHost
        int OutboundPort
        int TaskConcurrency
        int GlobalConcurrency
    }

    SIP_CAMPAIGN_CONTACT {
        uint ID PK
        uint CampaignID FK
        string Phone
        string RequestURI
        string Display
        string Status
        int Priority
        int AttemptCount
        int MaxAttempts
        time NextRunAt
        json Variables
    }

    SIP_CALL_ATTEMPT {
        uint ID PK
        uint CampaignID FK
        uint ContactID FK
        int AttemptNo
        string CallID
        string State
        int SIPStatusCode
        string FailureReason
        time DialedAt
        time AnsweredAt
        time EndedAt
    }

    SIP_SCRIPT_RUN {
        uint ID PK
        uint CampaignID FK
        uint ContactID FK
        uint AttemptID FK
        string CallID
        string ScriptID
        string StepID
        string StepType
        string Result
        string InputText
        string OutputText
        int DurationMs
    }

    SIP_CAMPAIGN_EVENT {
        uint ID PK
        uint CampaignID FK
        uint ContactID FK
        uint AttemptID FK
        string Type
        string Level
        string Message
        json Meta
    }

    %% ACD 路由
    ACD_POOL_TARGET {
        uint ID PK
        uint TenantID FK
        string Name
        string RouteType
        string TargetValue
        string SipSource
        string SipTrunkHost
        int SipTrunkPort
        int Weight
        string WorkState
        time WorkStateAt
        time WebSeatLastSeenAt
    }

    ACD_SHIFT_SCHEDULE {
        uint ID PK
        uint TargetID FK
        int Weekday
        time StartTime
        time EndTime
    }

    %% 资源管理
    TRUNK {
        uint ID PK
        uint TenantID FK
        string Name
        string ProviderCode UK
        string Prefix
        string LocalAddr
    }

    TRUNK_NUMBER {
        uint ID PK
        uint TrunkID FK
        string Number
        string Direction
        string Status
        uint Concurrent
        bool IsTransferRelay
    }

    SIP_SCRIPT_TEMPLATE {
        uint ID PK
        uint TenantID FK
        string Name
        string ScriptID
        string Version
        string Description
        bool Enabled
        json ScriptSpec
    }

    CREDENTIAL {
        uint ID PK
        uint TenantID FK
        string Name
        string AccessKey UK
        string SecretKey
        string Status
        string AllowIP
    }

    %% 关系定义
    BASEMODEL ||--o{ TENANT : extends
    BASEMODEL ||--o{ TENANT_USER : extends
    BASEMODEL ||--o{ TENANT_GROUP : extends
    BASEMODEL ||--o{ TENANT_ROLE : extends
    BASEMODEL ||--o{ SIP_CAMPAIGN : extends
    BASEMODEL ||--o{ ACD_POOL_TARGET : extends

    TENANT ||--o{ TENANT_USER : contains
    TENANT ||--o{ TENANT_GROUP : contains
    TENANT ||--o{ TENANT_ROLE : contains
    TENANT ||--o{ SIP_CAMPAIGN : owns
    TENANT ||--o{ TRUNK : owns
    TENANT ||--o{ SIP_SCRIPT_TEMPLATE : owns
    TENANT ||--o{ CREDENTIAL : owns
    TENANT ||--o{ ACD_POOL_TARGET : owns

    TENANT_USER ||--o{ TENANT_USER_GROUP : belongs_to
    TENANT_GROUP ||--o{ TENANT_USER_GROUP : has

    TENANT_USER ||--o{ TENANT_USER_ROLE : has
    TENANT_ROLE ||--o{ TENANT_USER_ROLE : assigned_to

    TENANT_ROLE ||--o{ TENANT_ROLE_PERMISSION : has
    PERMISSION ||--o{ TENANT_ROLE_PERMISSION : assigned_to

    SIP_CAMPAIGN ||--o{ SIP_CAMPAIGN_CONTACT : contains
    SIP_CAMPAIGN ||--o{ SIP_CALL_ATTEMPT : tracks
    SIP_CAMPAIGN ||--o{ SIP_SCRIPT_RUN : records
    SIP_CAMPAIGN ||--o{ SIP_CAMPAIGN_EVENT : logs

    SIP_CAMPAIGN_CONTACT ||--o{ SIP_CALL_ATTEMPT : generates
    SIP_CAMPAIGN_CONTACT ||--o{ SIP_SCRIPT_RUN : triggers

    TRUNK ||--o{ TRUNK_NUMBER : has

    ACD_POOL_TARGET ||--o{ ACD_SHIFT_SCHEDULE : schedules
```

## 模块分层架构

```mermaid
flowchart TB
    subgraph "表现层"
        UI[Web UI / API Clients]
    end

    subgraph "API 网关层"
        AG[Handlers / Controllers<br/>- tenant_users.go<br/>- sip_campaigns.go<br/>- trunks.go<br/>- ...]
    end

    subgraph "业务逻辑层"
        BL[Service Layer<br/>- sipserver.CampaignService<br/>- ACD Router<br/>- Auth Middleware]
    end

    subgraph "数据访问层 (Models)"
        direction TB

        subgraph "RBAC 模块"
            RBAC[Tenant + User + Role + Permission]
        end

        subgraph "外呼活动模块"
            OB[SIPCampaign + Contact + Attempt + ScriptRun]
        end

        subgraph "ACD 路由模块"
            ACDM[ACDPoolTarget + ShiftSchedule]
        end

        subgraph "资源管理模块"
            RES[Trunk + Number + ScriptTemplate + Credential]
        end
    end

    subgraph "数据存储层"
        DB[(PostgreSQL/MySQL)]
        CACHE[(Redis Cache)]
    end

    UI --> AG
    AG --> BL
    BL --> RBAC
    BL --> OB
    BL --> ACDM
    BL --> RES
    RBAC --> DB
    OB --> DB
    ACDM --> DB
    RES --> DB
```

## 核心领域模型关系

### 1. 多租户 RBAC 领域

```mermaid
classDiagram
    class Tenant {
        +uint ID
        +string Name
        +string Slug
        +string Status
        +Create()
        +GetBySlug()
    }

    class TenantUser {
        +uint ID
        +uint TenantID
        +string Email
        +string Phone
        +string Username
        +string PasswordHash
        +string Status
        +ListPage()
        +GetByID()
        +Create()
        +Update()
        +SoftDelete()
    }

    class TenantGroup {
        +uint ID
        +uint TenantID
        +string Name
        +bool IsDefault
    }

    class TenantRole {
        +uint ID
        +uint TenantID
        +string Name
        +bool IsSystem
        +Create()
        +GetByName()
    }

    class Permission {
        +uint ID
        +string Code
        +string Name
        +string Resource
        +string Action
    }

    class PlatformAdmin {
        +uint ID
        +string Email
        +string PasswordHash
        +string Status
        +GetByEmail()
    }

    Tenant "1" --> "*" TenantUser : contains
    Tenant "1" --> "*" TenantGroup : contains
    Tenant "1" --> "*" TenantRole : contains
    TenantUser "*" --> "*" TenantGroup : belongs
    TenantUser "*" --> "*" TenantRole : has
    TenantRole "*" --> "*" Permission : grants
```

### 2. SIP 外呼活动领域

```mermaid
classDiagram
    class SIPCampaign {
        +uint ID
        +uint TenantID
        +string Name
        +string Status
        +string Scenario
        +json ScriptSpec
        +int MaxAttempts
        +int TaskConcurrency
        +Start()
        +Pause()
        +Stop()
        +AddContacts()
    }

    class SIPCampaignContact {
        +uint ID
        +uint CampaignID
        +string Phone
        +string Status
        +int AttemptCount
        +json Variables
    }

    class SIPCallAttempt {
        +uint ID
        +uint CampaignID
        +uint ContactID
        +int AttemptNo
        +string State
        +int SIPStatusCode
        +time DialedAt
    }

    class SIPScriptRun {
        +uint ID
        +uint CampaignID
        +uint ContactID
        +string StepID
        +string Result
        +string OutputText
    }

    class SIPCampaignEvent {
        +uint ID
        +uint CampaignID
        +string Type
        +string Level
        +string Message
    }

    SIPCampaign "1" --> "*" SIPCampaignContact : contains
    SIPCampaign "1" --> "*" SIPCallAttempt : tracks
    SIPCampaign "1" --> "*" SIPScriptRun : records
    SIPCampaign "1" --> "*" SIPCampaignEvent : logs
    SIPCampaignContact "1" --> "*" SIPCallAttempt : generates
```

### 3. ACD 路由领域

```mermaid
classDiagram
    class ACDPoolTarget {
        +uint ID
        +uint TenantID
        +string RouteType
        +string TargetValue
        +string SipSource
        +int Weight
        +string WorkState
        +time WebSeatLastSeenAt
        +PickTarget()
        +UpdateWorkState()
    }

    class ACDShiftSchedule {
        +uint ID
        +uint TargetID
        +int Weekday
        +time StartTime
        +time EndTime
        +IsInShift()
    }

    class Trunk {
        +uint ID
        +uint TenantID
        +string Name
        +string ProviderCode
        +[]TrunkNumber Numbers
    }

    class TrunkNumber {
        +uint ID
        +uint TrunkID
        +string Number
        +string Status
        +uint Concurrent
    }

    ACDPoolTarget "1" --> "*" ACDShiftSchedule : schedules
    Trunk "1" --> "*" TrunkNumber : has
    ACDPoolTarget --> Trunk : routes_to
```

## 数据流向图

```mermaid
sequenceDiagram
    participant C as Client
    participant H as Handler
    participant M as Model
    participant DB as Database

    %% 创建租户用户流程
    rect rgb(200, 230, 250)
        Note over C,DB: 创建租户用户流程
        C->>H: POST /tenant-users
        H->>M: CheckTenantUserEmailExists()
        M->>DB: SELECT count(*)
        DB-->>M: exists: false
        M-->>H: false
        H->>M: CreateTenantUser()
        M->>DB: INSERT tenant_users
        DB-->>M: user created
        M-->>H: success
        H-->>C: 201 Created
    end

    %% 创建外呼活动流程
    rect rgb(250, 230, 200)
        Note over C,DB: 创建外呼活动流程
        C->>H: POST /sip-center/campaigns
        H->>M: CreateSIPCampaign()
        M->>DB: INSERT sip_campaigns
        DB-->>M: campaign created
        M-->>H: success
        H-->>C: 201 Created
    end

    %% 添加联系人流程
    rect rgb(230, 250, 200)
        Note over C,DB: 添加活动联系人流程
        C->>H: POST /campaigns/:id/contacts
        H->>M: GetActiveSIPCampaignByID()
        M->>DB: SELECT * FROM sip_campaigns
        DB-->>M: campaign
        H->>M: BuildSIPCampaignContactsBatch()
        M->>DB: INSERT sip_campaign_contacts
        DB-->>M: contacts created
        M-->>H: success
        H-->>C: 200 OK
    end

    %% ACD 路由选择流程
    rect rgb(250, 200, 230)
        Note over C,DB: ACD 转人工路由流程
        C->>H: DTMF *0 (Transfer Request)
        H->>M: PickACDTarget()
        M->>DB: SELECT * FROM acd_pool_targets<br/>WHERE work_state='available'<br/>ORDER BY weight DESC
        DB-->>M: available targets
        M->>M: FilterByShiftSchedule()
        M->>M: WebSeatLastSeenFresh()
        M-->>H: selected target
        H-->>C: Transfer to target
    end
```

## 表结构汇总

| 模块 | 表名 | 说明 |
|------|------|------|
| 基础 | base_model | 所有模型的基类字段 |
| RBAC | tenants | 租户表 |
| RBAC | tenant_users | 租户用户表 |
| RBAC | tenant_groups | 租户组表 |
| RBAC | tenant_user_groups | 用户-组关联表 |
| RBAC | tenant_roles | 租户角色表 |
| RBAC | tenant_user_roles | 用户-角色关联表 |
| RBAC | permissions | 权限表 |
| RBAC | tenant_role_permissions | 角色-权限关联表 |
| RBAC | platform_admins | 平台管理员表 |
| 外呼 | sip_campaigns | 外呼活动表 |
| 外呼 | sip_campaign_contacts | 活动联系人表 |
| 外呼 | sip_call_attempts | 呼叫尝试记录表 |
| 外呼 | sip_script_runs | 脚本执行记录表 |
| 外呼 | sip_campaign_events | 活动事件日志表 |
| ACD | acd_pool_targets | ACD池目标表 |
| ACD | acd_shift_schedules | 班次调度表 |
| 资源 | sip_trunks | 线路表 |
| 资源 | sip_trunk_numbers | 线路号码表 |
| 资源 | sip_script_templates | 脚本模板表 |
| 资源 | credentials | API凭证表 |
