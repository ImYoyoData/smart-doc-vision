export interface TourStep {
  /** CSS selector for the element to spotlight. Empty means a centered card. */
  target?: string
  title: string
  /** Spoken aloud and shown in the tooltip. */
  body: string
  /** Preferred tooltip side; the overlay flips automatically when it does not fit. */
  placement?: 'top' | 'bottom' | 'left' | 'right'
}

export interface TourDefinition {
  key: string
  title: string
  steps: TourStep[]
}

const basic: TourDefinition = {
  key: 'basic',
  title: '新手引导',
  steps: [
    {
      title: '欢迎使用智审图档',
      body: '这是一段约一分钟的基础引导，会带你认识左侧导航、顶部工具栏和各个功能区。每一步都可以点「下一步」继续，也可以随时跳过。',
    },
    {
      target: '.project-switch',
      title: '当前项目',
      body: '所有模型接入、审核配置和审核任务都按项目隔离。开始工作前，先在这里选择或切换到目标项目。',
    },
    {
      target: '.nav',
      title: '左侧功能导航',
      body: '左侧按工作台、审核中心、资源配置、安全治理、系统管理分组。导航里只会显示当前账号有权限访问的功能。',
    },
    {
      target: '.nav',
      title: '工作台：仪表盘与数据报表',
      body: '仪表盘汇总今天的审核量、待人工复核和最近任务；数据报表按项目、审核配置和时间范围分析通过率、驳回原因与延迟分布。',
    },
    {
      target: '.nav',
      title: '审核中心：待议复核、审核任务、审核配置',
      body: '模型无法确定的图档会进入待议复核；审核任务记录每一次提交与结论；审核配置把媒体类型、模型、提示词和规则阈值编排成可复用的审核流程。',
    },
    {
      target: '.nav',
      title: '资源配置：模型接入与数据连接',
      body: '模型接入管理通用大模型和 JEV 决策模型，支持获取模型列表和一键测试；数据连接、通知渠道、接入凭证和项目空间也在这里维护。',
    },
    {
      target: '.topbar-right',
      title: '顶部工具栏',
      body: '右上角可以切换项目、查看站内消息、打开对接文档，以及随时点「引导帮助」重看这段引导。',
    },
    {
      target: '.topbar-tour',
      title: '引导帮助',
      body: '以后想再看基础引导就点这个图标。每个功能页面的标题右侧也有一个引导按钮，会针对当前页面做详细讲解。',
    },
    {
      title: '基础引导完成',
      body: '你可以随时在任意页面点击标题旁的小引导图标，查看该页面的详细说明。现在开始使用吧。',
    },
  ],
}

const pageTours: Record<string, TourDefinition> = {
  dashboard: {
    key: 'dashboard',
    title: '仪表盘引导',
    steps: [
      { target: '.filter-bar', title: '筛选条件', body: '在这里选择项目和审核配置，仪表盘的所有数据都会按筛选条件实时更新。' },
      { target: '.hero', title: '今日概览', body: '顶部展示今天提交量、通过率与待处理数量，帮助你快速判断当前工作量。' },
      { target: '.metric-grid', title: '关键指标', body: '通过、驳回、待复核、失败等指标卡片，点击可以跳转到对应的任务列表。' },
      { target: '.ops-grid', title: '待复核队列与最近任务', body: '左侧按等待时间列出待人工复核的任务，右侧显示最近提交及结论，方便快速跟进。' },
    ],
  },
  analytics: {
    key: 'analytics',
    title: '数据报表引导',
    steps: [
      { target: '.filter-panel', title: '筛选与时间范围', body: '切换项目、审核配置和今日、近七天、近三十天、近九十天，图表与指标会同步刷新。' },
      { target: '.metric-grid', title: '核心指标', body: '审核总量、通过率、驳回量与平均耗时集中展示，作为后续图表的解读基准。' },
      { target: '.main-charts', title: '趋势图', body: '按日期堆叠展示通过、驳回和待议的数量变化，用来发现异常波动。' },
    ],
  },
  reviews: {
    key: 'reviews',
    title: '待议复核引导',
    steps: [
      { target: '.review-banner', title: '复核提醒', body: '这里显示等待复核的总量和按审核配置的分布，点击标签可以快速筛选。' },
      { target: '.review-queue', title: '复核队列', body: '左侧队列按进入时间升序排列，支持按审核配置、状态和日期筛选。' },
      { target: '.review-analysis', title: '模型判定依据', body: '右侧展示模型给出的结论、风险分、命中规则和原始输入，帮助你判断是否改判。' },
      { target: '.review-action-bar', title: '提交复核结论', body: '选择通过、驳回或继续转人工并填写说明，提交后会触发结果路由并回调业务方。' },
    ],
  },
  tasks: {
    key: 'tasks',
    title: '审核任务引导',
    steps: [
      { target: '.task-toolbar', title: '筛选与搜索', body: '按项目、审核配置、状态、结论、来源和时间筛选任务，也可以直接搜索任务号或业务键。' },
      { target: '.data-table-wrap', title: '任务列表', body: '每一行是一条审核任务。点击任务可以查看输入内容、模型原始返回、规则命中过程和事件时间线。' },
    ],
  },
  audits: {
    key: 'audits',
    title: '审核配置引导',
    steps: [
      { target: '.audit-summary', title: '配置概览', body: '展示启用中的审核配置、分类器数量与今日调用量，帮助你掌握整体运行情况。' },
      { target: '.audit-toolbar', title: '搜索与新建', body: '搜索审核名称或编码；点「新建审核配置」可以从模板开始，也可以让 AI 配置助手根据描述生成草稿。' },
      { target: '.audit-list', title: '配置卡片', body: '每张卡片是一套审核流程：媒体类型、绑定的模型、审核要求、规则阈值与触发方式都在这里维护。' },
    ],
  },
  providers: {
    key: 'providers',
    title: '模型接入引导',
    steps: [
      { target: '.provider-summary', title: '状态概览', body: '按已启用、可用、不可用和已停用筛选模型，右侧可以一键测试全部已启用连接。' },
      { target: '.provider-grid', title: '模型卡片', body: '每张卡片展示协议、上下文、能力与请求用量。编辑时可以重新获取模型列表，自动带入上下文与最大输出。' },
    ],
  },
  users: {
    key: 'users',
    title: '用户与权限引导',
    steps: [
      { target: '.security-banner', title: '权限模型', body: '账号权限由角色权限和项目数据范围共同决定：角色决定能做什么，项目范围决定能看哪些数据。' },
      { target: '.panel', title: '账号与角色', body: '在这里创建后台账号、分配角色、重置密码与两步验证，也可以自定义角色可访问的菜单和操作。' },
    ],
  },
  monitor: {
    key: 'monitor',
    title: '运行监控引导',
    steps: [
      { target: '.monitor-metrics', title: '进程指标', body: '展示运行时间、进程常驻内存、协程与线程、请求量和平均耗时；空闲时会自动把堆内存还给系统。' },
      { target: '.monitor-grid', title: '趋势与依赖', body: '图表展示内存、并发和响应耗时趋势，下方列出 MySQL 与 Redis 连接池状态和项目 Worker 队列。' },
    ],
  },
  projects: {
    key: 'projects',
    title: '项目空间引导',
    steps: [
      { target: '.project-summary', title: '项目概览', body: '汇总项目数量、启用状态、每日任务配额与保留天数，帮助你了解资源占用。' },
      { target: '.project-toolbar', title: '搜索与新建', body: '搜索项目名称或编码；新建项目时可以设置并发上限、每日配额、保留天数和脚本沙箱限制。' },
      { target: '.project-grid', title: '项目卡片', body: '项目是配置、模型、任务与数据权限的隔离边界。切换当前项目会同时切换侧边栏和所有页面的数据范围。' },
    ],
  },
  credentials: {
    key: 'credentials',
    title: '接入凭证引导',
    steps: [
      { target: '.credential-summary', title: '凭证概览', body: '展示凭证数量与最近调用情况，帮助你确认哪些业务方已经接入。' },
      { target: '.credential-grid', title: 'AppKey 管理', body: '每个凭证拥有独立的 AppKey 和 AppSecret，并可按审核配置精确授权访问范围，支持轮换与停用。' },
    ],
  },
  connections: {
    key: 'connections',
    title: '数据连接引导',
    steps: [
      { target: '.connection-grid', title: 'Redis 连接', body: '这里维护项目级 Redis 连接，供审核触发与结果动作复用；平台自身缓存使用的 Redis 与它们相互独立。' },
    ],
  },
  notifications: {
    key: 'notifications',
    title: '通知渠道引导',
    steps: [
      { target: '.channel-banner', title: '通知说明', body: '通知渠道用于审核完成、复核提醒和系统告警的对外发送，目前支持邮件。' },
      { target: '.channel-grid', title: '渠道卡片', body: '每张卡片是一个发送账号，可单独测试连通性并启用或停用。' },
    ],
  },
  logs: {
    key: 'logs',
    title: '审计日志引导',
    steps: [
      { target: '.log-toolbar', title: '日志筛选', body: '按操作人、动作、对象、结果和时间范围筛选，支持按 trace_id 串联同一次请求的全部操作。' },
      { target: '.data-table-wrap', title: '日志明细', body: '点击任意一行可以查看完整详情，包括请求参数、变更前后差异与来源 IP。' },
    ],
  },
  recycle: {
    key: 'recycle',
    title: '回收站引导',
    steps: [
      { target: '.recycle-toolbar', title: '回收站说明', body: '删除的项目与审核配置会先进入回收站，在保留期内可以恢复。' },
      { target: '.data-table-wrap', title: '回收列表', body: '展开项目可以查看其下的审核配置；保留期结束后会自动永久清理，超级管理员也可以立即永久删除。' },
    ],
  },
  'system-config': {
    key: 'system-config',
    title: '系统配置引导',
    steps: [
      { target: '.config-banner', title: '只读配置说明', body: '这里展示进程启动时读取的配置文件与来源；修改 .env 后需要重启服务才会生效。' },
      { target: '.config-grid', title: '配置分组', body: '运行环境、数据库、Redis、安全密钥、文件存储等分组集中展示，密码和密钥均已脱敏。' },
    ],
  },
  help: {
    key: 'help',
    title: '对接文档引导',
    steps: [
      { target: '.help-layout', title: '对接文档', body: '左侧是目录，右侧是正文，涵盖鉴权、三种触发方式、任务参数、WebSocket 协议与回调签名说明。' },
    ],
  },
  messages: {
    key: 'messages',
    title: '站内消息引导',
    steps: [
      { target: '.page-head', title: '站内消息', body: '这里汇总审核完成、复核提醒和系统通知。点开任意一条可以查看详情并一键跳转到对应任务。' },
    ],
  },
  profile: {
    key: 'profile',
    title: '个人中心引导',
    steps: [
      { target: '.page-head', title: '个人中心', body: '这里查看账号身份与项目范围，并可以修改登录密码、管理两步验证。' },
    ],
  },
}

// Form tours open inside a modal, so their targets are the data-tour markers on
// the modal's own sections rather than page-level selectors. Each form asks once
// when it is first opened, then never again.
const formTours: Record<string, TourDefinition> = {
  'provider-form': {
    key: 'provider-form',
    title: '新增模型接入引导',
    steps: [
      {
        target: '[data-tour="provider-preset"]',
        title: '快速选择供应商',
        body: '先在这里选择供应商，协议、BaseURL 和推荐的起始模型会自动填好；选择后下方会显示该供应商的说明和官方文档链接。只想手动填写时可以直接跳过这一项。',
      },
      {
        target: '[data-tour="provider-kind"]',
        title: '选择接入类型',
        body: '通用大模型包含 OpenAI 兼容与 Anthropic 两种协议，业务上属于同一类，审核时可以互相回落；JEV 是独立的结构化决策模型，不生成自由文本。协议在创建后不能修改。',
      },
      {
        target: '[data-tour="provider-model"]',
        title: '模型 ID 与能力',
        body: '填写 BaseURL 与 API Key 后点「获取模型」，下拉里会列出供应商可用的模型。选中后会自动带入上下文窗口、最大输出、是否支持图片和思考，并标明能力来源；这些数值仍可手动调整。',
      },
      {
        target: '[data-tour="provider-params"]',
        title: '推理参数',
        body: '这里设置最大图片数、上下文窗口、最大输出 Token 与温度。支持推理的模型还可以打开「优先使用推理」，失败回落时同类模型会优先选择支持推理的。',
      },
      {
        target: '[data-tour="provider-footer"]',
        title: '保存并测试',
        body: '保存后回到列表，点卡片上的「测试连接」确认密钥和模型可用。新接入默认是停用状态，测试通过再手动启用，避免污染审核链路。',
      },
    ],
  },
  'audit-form': {
    key: 'audit-form',
    title: '新增审核配置引导',
    steps: [
      {
        target: '[data-tour="audit-basic"]',
        title: '名称与编码',
        body: '审核名称用于后台展示；审核编码是开放 API 调用和回调里的唯一标识，创建后不能修改，建议使用小写字母加下划线的业务含义命名。',
      },
      {
        target: '[data-tour="audit-mode"]',
        title: '审核类型',
        body: '复核模式由模型直接给出通过、驳回或转人工；分类模式先选分类再按映射得到结论；纯分类器只返回分类类型，成功审核固定为通过。分类类场景建议搭配 JEV。',
      },
      {
        target: '[data-tour="audit-provider"]',
        title: '模型供应商与媒体类型',
        body: '选择这条流程绑定的模型，失败时会按优先级回落到同项目其他已启用模型。媒体类型决定这条流程接收文本、图片还是两者都接收。',
      },
      {
        target: '[data-tour="audit-sections"]',
        title: '详细配置',
        body: '审核条约、审核要求、规则阈值、结果路由、脚本动作和触发方式都在这里按需展开编辑。每一项都会单独开弹窗，主表单只保留摘要。',
      },
      {
        target: '[data-tour="audit-footer"]',
        title: '保存与 AI 助手',
        body: '创建后会生成开放 API、Redis 和 WebSocket 三种接入方式并打开接入指南。也可以点「AI 帮我修改」，用自然语言描述要调整的地方。',
      },
    ],
  },
  'task-submit': {
    key: 'task-submit',
    title: '发起审核引导',
    steps: [
      {
        target: '[data-tour="submit-audit"]',
        title: '选择审核配置',
        body: '先选择这条任务要走的审核流程，会决定使用的模型、规则与结果路由。列表只显示当前项目下已启用的配置。',
      },
      {
        target: '[data-tour="submit-content"]',
        title: '填写业务键与内容',
        body: '业务键用于去重和业务关联；文档文本与图档图片至少提供一项。相同业务键加相同内容会在去重窗口内复用原任务。',
      },
      {
        target: '[data-tour="submit-callback"]',
        title: '回调地址与扩展信息',
        body: '回调地址会在审核完成时收到带签名的结果推送；扩展信息是透传给模型的补充上下文，用 JSON 填写。',
      },
      {
        target: '[data-tour="submit-actions"]',
        title: '提交任务',
        body: '提交后任务进入异步审核队列，可以在审核任务页看到实时状态，在待议复核页处理模型无法确定的图档。',
      },
    ],
  },
  'project-form': {
    key: 'project-form',
    title: '新建项目引导',
    steps: [
      {
        target: '[data-tour="project-basic"]',
        title: '项目名称与编码',
        body: '项目编码是配置、模型、任务与回调的隔离边界，创建后不能修改；环境标签用于区分开发、测试与生产。',
      },
      {
        target: '[data-tour="project-limits"]',
        title: '保留期与配额',
        body: '回收站保留天数决定删除后的恢复窗口；每日任务配额与并发 Worker 上限约束这个项目可以消耗的资源，并发还受全局 Worker 总数限制。',
      },
      {
        target: '[data-tour="project-footer"]',
        title: '创建并切换',
        body: '创建后会立即切换为当前项目，之后在这个项目下配置模型、审核与凭证。',
      },
    ],
  },
  'user-form': {
    key: 'user-form',
    title: '新建账号引导',
    steps: [
      {
        target: '[data-tour="user-basic"]',
        title: '账号与名称',
        body: '登录账号创建后不能修改；显示名称用于后台展示和水印。邮箱用于账号通知，可以留空。',
      },
      {
        target: '[data-tour="user-security"]',
        title: '密码与角色',
        body: '初始密码使用 bcrypt 存储；角色决定能访问哪些菜单和操作。编辑账号时密码留空表示不修改。',
      },
      {
        target: '[data-tour="user-scope"]',
        title: '项目与审核范围',
        body: '非超级管理员必须限定项目数据范围，还可以进一步限定可查看和提交的审核配置。不选审核配置表示账号项目范围内的全部配置。',
      },
      {
        target: '[data-tour="user-footer"]',
        title: '保存账号',
        body: '保存后账号即可登录。建议提醒对方首次登录后到个人中心开启两步验证。',
      },
    ],
  },
  'credential-form': {
    key: 'credential-form',
    title: '新建凭证引导',
    steps: [
      {
        target: '[data-tour="credential-name"]',
        title: '凭证名称',
        body: '用来区分调用方，例如「业务系统 A · 生产」。AppKey 与 AppSecret 生成后只显示一次，需要立即保存。',
      },
      {
        target: '[data-tour="credential-scope"]',
        title: '授权范围',
        body: '凭证只能访问这里勾选的审核配置。审核配置删除后授权自动失效，恢复后如果授权仍在，访问也会恢复。',
      },
      {
        target: '[data-tour="credential-footer"]',
        title: '创建凭证',
        body: '创建后会弹出一次 AppSecret，关闭后无法再次查看，只能重新轮换。',
      },
    ],
  },
  'connection-form': {
    key: 'connection-form',
    title: '新建数据连接引导',
    steps: [
      {
        target: '[data-tour="connection-name"]',
        title: '连接名称',
        body: '项目级 Redis 连接，供审核触发和结果动作复用；平台自身缓存使用的 Redis 与这里配置的相互独立。',
      },
      {
        target: '[data-tour="connection-addr"]',
        title: '地址与 DB',
        body: '填写 host:port，不写端口默认 6379；DB 编号是这条连接的默认库，触发源和结果动作仍可单独覆盖。',
      },
      {
        target: '[data-tour="connection-auth"]',
        title: '认证与 TLS',
        body: 'Redis 6 及以上可以填写 ACL 用户名；密码留空表示无密码。需要加密传输时打开 TLS。',
      },
      {
        target: '[data-tour="connection-footer"]',
        title: '保存并测试',
        body: '保存后点卡片上的测试，确认地址、密码与 DB 都可连通，再在触发源里引用这条连接。',
      },
    ],
  },
  'role-form': {
    key: 'role-form',
    title: '新建角色引导',
    steps: [
      {
        target: '[data-tour="role-basic"]',
        title: '角色编码与名称',
        body: '角色编码是权限判断使用的唯一标识，创建后不能修改；角色名称会显示在账号的权限标签上。',
      },
      {
        target: '[data-tour="role-permissions"]',
        title: '菜单与操作权限',
        body: '可见菜单决定左侧导航显示哪些页面，操作权限决定能执行哪些写操作。两者都需要勾选：只给菜单不给权限时，接口仍会拒绝访问。运行监控需要单独勾选「查看运行监控」。',
      },
      {
        target: '[data-tour="role-footer"]',
        title: '保存角色',
        body: '保存后可以在账号编辑里选择这个角色。改动角色权限会立即影响所有使用该角色的账号。',
      },
    ],
  },
  'channel-form': {
    key: 'channel-form',
    title: '新建通知渠道引导',
    steps: [
      {
        target: '[data-tour="channel-basic"]',
        title: '渠道名称与类型',
        body: '渠道名称用于在通知偏好里选择；目前支持邮箱（SMTP），后续会扩展短信与 Webhook。',
      },
      {
        target: '[data-tour="channel-smtp"]',
        title: 'SMTP 与发件人',
        body: '选择服务商会自动带出主机与加密方式；发件邮箱通常需要与 SMTP 账号一致，否则容易被判为伪造。',
      },
      {
        target: '[data-tour="channel-footer"]',
        title: '保存并测试',
        body: '保存后点「测试发送」确认能收到邮件，再到个人中心或审核配置里指定使用该渠道。',
      },
    ],
  },
}

export const TOURS: Record<string, TourDefinition> = { basic, ...pageTours, ...formTours }

export function getTour(key: string): TourDefinition | undefined {
  return TOURS[key]
}
