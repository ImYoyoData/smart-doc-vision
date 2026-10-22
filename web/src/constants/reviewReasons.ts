export const REVIEW_REASON_OPTIONS = [
  { label: '人工确认', value: 'MANUAL_CONFIRMED' },
  { label: '风险确认', value: 'RISK_CONFIRMED' },
  { label: '分类不确定', value: 'CLASSIFICATION_UNCERTAIN' },
  { label: '证据不足 / 内容不完整', value: 'INCOMPLETE_EVIDENCE' },
  { label: '模型异常', value: 'MODEL_ERROR' },
  { label: '规则转人工', value: 'RULE_REVIEW' },
  { label: '敏感信息 / 合规风险', value: 'SENSITIVE_INFO' },
  { label: '其他', value: 'OTHER' },
]
