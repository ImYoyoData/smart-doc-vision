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

export const TOURS: Record<string, TourDefinition> = { basic, ...pageTours }

export function getTour(key: string): TourDefinition | undefined {
  return TOURS[key]
}
