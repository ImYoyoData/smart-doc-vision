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
      {
        target: '[data-tour="dash-filter"]',
        title: '筛选条件',
        body: '先在这里选择项目和审核配置。仪表盘顶部概览、指标卡和下方两个队列都会按筛选条件实时重算；切换项目时其他页面的数据范围也会一起切换。',
      },
      {
        target: '[data-tour="dash-hero"]',
        title: '今日概览',
        body: '这块展示今天的提交量、通过率、待复核数量和平均耗时，用来快速判断今天的工作量是否正常。数字只统计今天零点到现在。',
      },
      {
        target: '.review-alert',
        title: '待复核提醒',
        body: '有待人工复核的任务时，这里会出现提醒条。点「进入复核工作台」直接跳到复核队列，按等待时间从早到晚处理。',
      },
      {
        target: '[data-tour="dash-metrics"]',
        title: '关键指标',
        body: '通过、驳回、待复核、执行失败四张卡片。点击卡片会跳到对应状态的任务列表，方便核对具体是哪些任务。',
      },
      {
        target: '[data-tour="dash-ops"]',
        title: '待复核队列与最近任务',
        body: '左侧按等待时间升序列出待人工复核的任务，点击任意一条进入复核详情；右侧展示最近提交的任务及其结论，用来确认最新的审核是否按预期工作。',
      },
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
      {
        target: '.review-banner',
        title: '复核提醒与筛选',
        body: '顶部显示当前等待复核的总量，以及按审核配置的分布。点击某个审核配置的标签，可以只看这条流程转来的任务。',
      },
      {
        target: '.review-queue',
        title: '复核队列',
        body: '左侧队列按进入时间升序排列，最早等待的排在最前。支持按审核配置、结论状态和日期范围筛选，处理完当前任务后会自动选中下一条。',
      },
      {
        target: '.review-analysis',
        title: '模型判定依据',
        body: '右侧展示模型给出的结论、风险分、分类结果、命中规则和原因。请先核对这里的依据再改判，避免仅凭直觉覆盖模型判断。',
      },
      {
        target: '.review-action-bar',
        title: '提交复核结论',
        body: '选择通过、驳回或继续转人工，并填写复核说明。提交后会触发结果路由、推送回调和站内通知；复核记录会写入审计日志，可在任务详情里追溯。',
      },
    ],
  },
  tasks: {
    key: 'tasks',
    title: '审核任务引导',
    steps: [
      {
        target: '[data-tour="tasks-toolbar"]',
        title: '筛选、搜索与列设置',
        body: '按审核配置、状态、结论、来源和时间范围筛选，也可以直接搜索任务号、业务键或 request_id。右侧的列设置可以隐藏不关心的字段，筛选组合还能保存为常用视图。',
      },
      {
        target: '[data-tour="tasks-table"]',
        title: '任务列表',
        body: '每一行是一条审核任务，展示提交时间、结论、风险分、命中的模型与耗时。点开任意一行会打开详情抽屉，能看到模型原始返回、规则命中过程、输入内容和完整事件时间线。',
      },
      {
        title: '发起审核与导出',
        body: '右上角「发起审核」可以直接在后台提交一次任务用于联调；列表支持按当前筛选条件导出 CSV，导出前请确认筛选范围，避免导出过多数据。',
      },
    ],
  },
  audits: {
    key: 'audits',
    title: '审核配置引导',
    steps: [
      {
        target: '[data-tour="audits-summary"]',
        title: '配置概览',
        body: '展示启用中的审核配置数量、分类器数量与今日调用量。这里有数字说明审核链路正在运行；长期为 0 说明还没有业务方调用。',
      },
      {
        target: '[data-tour="audits-toolbar"]',
        title: '搜索与新建',
        body: '按审核名称或编码搜索。点「新建审核配置」从零开始或套用模板；也可以打开 AI 配置助手，用自然语言描述需求，由模型生成完整草稿后人工确认。',
      },
      {
        target: '[data-tour="audits-list"]',
        title: '配置卡片',
        body: '每张卡片是一套完整的审核流程：媒体类型、绑定的模型、审核要求、条约、规则组、结果动作和触发方式。卡片上的状态开关决定它能否被任务和开放 API 使用。',
      },
      {
        title: '接入与复制',
        body: '卡片菜单里可以打开「接入指南」，查看这条流程的三种接入方式和签名说明；「复制」会连同规则、路由和提示词一起复制出一份草稿，适合快速搭建相似流程。',
      },
    ],
  },
  providers: {
    key: 'providers',
    title: '模型接入引导',
    steps: [
      {
        target: '[data-tour="providers-summary"]',
        title: '状态概览与批量测试',
        body: '按已启用、可用、不可用和已停用筛选。右侧「测试已启用连接」会并发测试当前筛选出的全部启用模型，适合在更换密钥或调整网络后确认整体可用性。',
      },
      {
        target: '[data-tour="providers-grid"]',
        title: '模型卡片',
        body: '每张卡片展示协议、模型 ID、上下文、图片与思考能力、优先级和累计请求量。审核绑定的模型始终最先调用，失败后同项目其他启用模型按优先级依次回落。',
      },
      {
        title: '新增与编辑',
        body: '点「新增模型接入」可以先用「快速选择供应商」自动填好协议、BaseURL 和主流模型，再填 API Key 并点「获取模型」带入真实能力参数。新接入默认停用，测试通过后再启用。',
      },
      {
        title: '熔断与恢复',
        body: '连续失败的模型会进入熔断冷却，卡片上会显示剩余时间。后台每 30 秒自动探测到期模型，手动「测试连接」成功也会立即清除熔断。',
      },
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
      {
        target: '[data-tour="profile-identity"]',
        title: '账号身份与项目范围',
        body: '这里显示登录账号、显示名称、角色和可以访问的项目范围。角色决定菜单与操作权限，项目范围决定能查看哪些项目、审核配置和任务。',
      },
      {
        target: '[data-tour="profile-password"]',
        title: '修改登录密码',
        body: '密码至少 10 位且包含字母和数字。修改后当前会话保持有效，其他已登录会话会在令牌过期后失效。',
      },
      {
        target: '[data-tour="profile-mfa"]',
        title: '两步验证',
        body: '启用后登录需要再输入身份验证器生成的 6 位动态验证码，登录页也可以直接用验证器登录。手机丢失时请联系超级管理员重置。',
      },
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
  'audit-prompt': {
    key: 'audit-prompt',
    title: '审核要求引导',
    steps: [
      {
        target: '[data-tour="prompt-actions"]',
        title: '插入条约与 AI 优化',
        body: '「插入条约占位符」会把已生效的审核条约插到光标位置，用 {{AUDIT_TREATY}} 指定准确落点；「AI 优化提示词」会根据你的描述改写当前要求，结果需要你确认后才会写回。',
      },
      {
        target: '[data-tour="prompt-text"]',
        title: '填写审核要求',
        body: '写清楚要审核什么、什么情况通过、什么情况驳回、什么情况转人工。这段文字会作为通用模型的审核 Prompt，也会作为 JEV 的审核要求一并发送。',
      },
    ],
  },
  'audit-treaty': {
    key: 'audit-treaty',
    title: '审核条约引导',
    steps: [
      {
        target: '[data-tour="treaty-banner"]',
        title: '条约如何生效',
        body: '上传的条约由服务器后台解析，关闭网页不会中断。解析成功后条约会自动追加到审核要求里，也可以在前面用占位符指定插入位置。只有解析成功的条约才参与判定。',
      },
      {
        target: '[data-tour="treaty-table"]',
        title: '条约列表',
        body: '表格展示每份条约的解析状态、字符数和生效情况。解析失败时可以查看失败原因并重新解析；失效的条约不会影响审核，但会保留在列表里方便排查。',
      },
      {
        title: '格式与限制',
        body: '支持 TXT、Markdown、DOCX，旧版 .doc 请先另存为 .docx，单文件不超过 20MB，每个审核配置最多 30 份。条约内容过长会自动截断，建议只保留与判定直接相关的条款。',
      },
    ],
  },
  'audit-category': {
    key: 'audit-category',
    title: '分类类型引导',
    steps: [
      {
        target: '[data-tour="category-panel"]',
        title: '分类与映射',
        body: '每一行是一个分类：编码会写入结果的 category 字段，说明是给模型看的判定依据。复核模式下每个分类还要指定映射结论；纯分类器模式下成功审核固定为通过，映射结论不生效。',
      },
      {
        title: '导入导出',
        body: '支持 CSV（code,name,description,result）和 JSON 数组导入，适合从现有分类表批量迁移；导出可用于备份或交给业务方确认。导入只影响当前审核配置。',
      },
    ],
  },
  'audit-rules': {
    key: 'audit-rules',
    title: '规则组引导',
    steps: [
      {
        target: '[data-tour="rules-toolbar"]',
        title: '模板与 AI 生成',
        body: '先选一个规则模板可以快速得到可用的阈值组合；「AI 生成规则」会根据你的描述产出规则 JSON，生成后仍需人工核对阈值是否符合业务预期。',
      },
      {
        target: '[data-tour="rules-editor"]',
        title: '规则组与条件',
        body: '规则组按优先级从上到下执行。每个条件可以是风险分、分类、标签或输入字段的比较；命中后可以选择继续执行后续规则或立即停止。全部未命中时使用运行策略里的默认结论。',
      },
    ],
  },
  'audit-routes': {
    key: 'audit-routes',
    title: '结果动作引导',
    steps: [
      {
        target: '[data-tour="routes-editor"]',
        title: '按结论配置动作',
        body: '分别给通过、驳回、转人工配置要执行的动作：HTTP 回调、Redis Stream 推送、写回业务数据库或运行 JS 脚本。没有配置动作时只写入平台结果，不会通知业务方。',
      },
      {
        title: '执行模式与失败处理',
        body: '「顺序执行」会按顺序跑完全部动作，「首个成功即停止」适合主备通道。失败处理决定某个动作失败时是继续下一个还是直接转人工；回调失败会按退避策略重试三次。',
      },
    ],
  },
  'audit-policy': {
    key: 'audit-policy',
    title: '运行策略引导',
    steps: [
      {
        target: '[data-tour="policy-basic"]',
        title: '默认结论与兜底',
        body: '默认结论是规则未命中时的结果；模型失败兜底决定所有模型都调用失败时按通过、驳回还是转人工处理。生产环境一般建议转人工，避免把故障当成放行。',
      },
      {
        target: '[data-tour="policy-dedupe"]',
        title: '人工复核与去重',
        body: '人工复核方式决定转人工的任务在平台内处理还是推给外部系统。去重窗口用于拦截重复提交：相同业务键加相同内容在窗口内会直接复用原任务，填 0 表示不去重。',
      },
    ],
  },
  'trigger-form': {
    key: 'trigger-form',
    title: '触发方式引导',
    steps: [
      {
        target: '[data-tour="trigger-basic"]',
        title: '名称、状态与优先级',
        body: '触发源名称用于区分不同业务队列；状态决定是否立即开始监听；优先级只影响列表展示顺序，不影响消息处理。',
      },
      {
        target: '[data-tour="trigger-connection"]',
        title: '选择 Redis 连接',
        body: '只能选择当前项目里已启用且测试成功的连接。没有可用连接时请先到「数据连接」新建并测试，否则触发源无法启动。',
      },
      {
        target: '[data-tour="trigger-stream"]',
        title: 'Stream 与消费组',
        body: 'Stream 是要监听的队列名。消费组用于多实例分摊消费，同一组内每条消息只会被一个实例处理；消费起点决定首次启动时从哪里开始读，批量大小和最大在途数控制吞吐与并发。',
      },
    ],
  },
  'task-detail': {
    key: 'task-detail',
    title: '任务详情引导',
    steps: [
      {
        target: '[data-tour="detail-hero"]',
        title: '审核结论',
        body: '顶部展示最终结论、风险分、命中的模型和耗时。结论可能来自模型判定、规则覆盖或人工复核，来源会在这里标明。',
      },
      {
        target: '[data-tour="detail-meta"]',
        title: '任务元信息',
        body: '业务键用于业务关联和去重，request_id 用于幂等与结果查询。排查问题时把这两个值提供给业务方，可以快速定位到同一次提交。',
      },
      {
        target: '[data-tour="detail-reasons"]',
        title: '审核依据',
        body: '这里列出模型给出的标签、原因和分类结果，下方还有规则命中过程与完整事件时间线。改判或复盘时以这些原始记录为准。',
      },
    ],
  },
  'integration-guide': {
    key: 'integration-guide',
    title: '接入指南引导',
    steps: [
      {
        target: '[data-tour="guide-identity"]',
        title: '项目与审核编码',
        body: '顶部显示这条流程所属的项目编码与审核编码。调用开放 API 时这两个值都要填在路径里，编码复制按钮可以直接取用。',
      },
      {
        target: '[data-tour="guide-tabs"]',
        title: '三种接入方式',
        body: '这里可以切换开放 API、Redis Stream 与 WebSocket 三种接入方式。每种方式都给出可直接复制的请求示例、字段说明和结果查询方法。',
      },
      {
        target: '[data-tour="guide-credential"]',
        title: '接入凭证',
        body: '调用需要项目下的 AppKey 与 AppSecret，并按审核配置授权。没有凭证时到「接入凭证」创建；回调验签使用凭证的 AppSecret 计算 HMAC-SHA256。',
      },
    ],
  },
  'script-editor': {
    key: 'script-editor',
    title: '结果脚本引导',
    steps: [
      {
        target: '[data-tour="script-toolbar"]',
        title: '编辑器工具栏',
        body: '上方可以切换脚本模板、格式化和查看执行日志。脚本在沙箱中运行，内存、超时、HTTP 次数与返回体积都受项目限制约束。',
      },
      {
        title: 'AI 生成脚本',
        body: '点右上角「AI」可以用自然语言描述要做什么，模型会读取当前脚本并生成修改建议，确认后才会写入编辑器。也可以用模拟运行验证脚本在给定结论下的行为。',
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
