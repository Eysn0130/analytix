export const USER_FACING_LANGUAGE_VERSION = "0.16.9-user-facing-language.1";

function text(value) {
  return String(value == null ? "" : value).trim();
}

const TERM_REPLACEMENTS = [
  [/集中转账簇|集中交易簇|集中簇/gu, "集中交易组"],
  [/其余链路|其余后续链路|其余去向/gu, "尚未固定的后续去向"],
  [/其余\s*(\d+\s*笔)/gu, "集中交易以外 $1"],
  [/其余数笔/gu, "尚未固定的数笔"],
  [/其余账号/gu, "尚未列明的账号"],
  [/簇外\s*(\d+\s*笔)/gu, "集中交易以外 $1"],
  [/簇外往来|簇外行/gu, "集中交易以外逐笔往来"],
  [/可疑簇/gu, "可疑线索集合"],
  [/时间簇/gu, "时间集中"],
  [/金额簇/gu, "金额集中"],
  [/账户簇/gu, "相关账户（列明账号）"],
  [/交易簇/gu, "交易组"],
  [/资金簇/gu, "资金集中流向"],
  [/\bfocus[_ -]?cluster\b/giu, "主要集中转账"],
  [/\bfocused\s+cluster\b|\bfocus\s+cluster\b/giu, "主要集中转账"],
  [/\boutside[_ -]?focus[_ -]?cluster\b|\boutside[- ]cluster\b/giu, "集中交易以外逐笔往来"],
  [/\bdominant[_ -]?(?:focus[_ -]?)?cluster\b|\blargest\s+cluster\b/giu, "主要集中转账"],
  [/\b(?:source[_ -]?account|account)[_ -]?clusters?\b/giu, "相关账户（列明账号）"],
  [/\b(?:time|date)[_ -]?clusters?\b/giu, "时间集中"],
  [/\bamount[_ -]?clusters?\b/giu, "金额集中"],
  [/\btransaction[_ -]?clusters?\b/giu, "交易集合"],
  [/\bclusters?\b/giu, "集中交易组"],
  [/\bquick-fact\b/giu, "资金事实快查"],
  [/\bSKILL\.md\b/giu, "专项规则"],
  [/\bList\s+MCP\s+resources\b/giu, "读取可用核验能力"],
  [/\bMCP\s+resources\b/giu, "可用核验能力"],
  [/\bShell\b/gu, "执行过程"],
  [/\bCodex\b/gu, "研判助手"],
  [/案件(?:编号|ID)\s*[:：]?\s*[0-9a-f][0-9a-f-]{7,}\b/giu, "当前案件"],
  [/\bcase[_\s-]*id\s*[:：]?\s*[0-9a-f][0-9a-f-]{7,}\b/giu, "当前案件"],
  [/(?:\/Users|\/var)\/[^\s"'，。；、)）]*\.(?:docx|xlsx|png|jpg|jpeg|pdf|txt)\b/gu, "已生成至 Analytix 交付区"],
  [/(?:\/Users|\/var)\/[^\s"'，。；、)）]+/gu, "本地路径已隐藏"],
  [/\b(?:cat|grep|find|ls)\s+本地路径已隐藏/giu, "内部检索过程已隐藏"],
  [/\bmcp__[\w.-]+\b/giu, "案件事实工具"],
  [/\bcreate_case_notebook\b/giu, "可回放核算记录"],
  [/\bexport_cleaned_case_data\b/giu, "清洗明细导出"],
  [/\brun_case_sql\b/giu, "专项资金核算"],
  [/\bControlled\s+Case\s+Workbench\b|\bControlled\s+Workbench\b|\bCase\s+Workbench\b|\bWorkbench\b/giu, "专项资金核算"],
  [/\bDuckDB\b/giu, "当前案件受控数据服务"],
  [/\bcase_id\b/giu, "案件编号"],
  [/\bquery_ids?\b/giu, "查询记录"],
  [/\bsource_hash\b/giu, "来源指纹"],
  [/\bmetric_scope\b/giu, "统计范围"],
  [/\braw_rows_exposed\b/giu, "明细展开状态"],
  [/\bcurrent_case_only\b/giu, "当前案件范围"],
  [/\bcleaned_analysis_scope_only\b/giu, "清洗后数据范围"],
  [/\bparser_binder_validated\b/giu, "查询预检"],
  [/\brow_limit\b/giu, "返回行数上限"],
  [/\bMermaid\b/giu, "资金流向图"],
  [/账户组/gu, "相关账户（需列明账号）"],
  [/收款端/gu, "收款账户"],
  [/主体账户集合/gu, "相关账户（需列明账号）"],
  [/本轮命中(?:的)?/gu, "已清洗流水显示的"],
  [/同事实去重(?:后(?:的)?金额|代表|支持|后)?/gu, "重复风险复核"],
  [/高可信追踪种子/gu, "可作为后续追查起点"],
  [/已有规范明细(?:\/分析图)?支持/gu, "已有流水支持"],
  [/证据状态/gu, "核验意见"],
  [/证据边界/gu, "核验意见"],
  [/证据状态[:：]\s*支持/gu, "已有流水支持"],
  [/已支持/gu, "已有流水支持"],
  [/姓名命中记录/gu, "姓名匹配的已清洗流水记录"],
  [/初筛命中总额|初步命中总额/gu, "初筛流水金额"],
  [/原始命中记录|原始匹配记录/gu, "初筛流水记录"],
  [/原命中明细|原始命中明细|原始匹配明细|命中明细/gu, "初筛流水明细"],
  [/命中记录/gu, "流水记录"],
  [/当前事实摘要未回传|当前摘要未回传|事实摘要未回传|摘要未回传|未回传/gu, "当前材料尚未取得"],
  [/当前事实摘要|当前摘要|当前摘录/gu, "当前材料"],
  [/本轮材料已(?:明确)?展示/gu, "现有材料已列明"],
  [/本轮已核到/gu, "现有材料已核实"],
  [/补齐本轮未展开的登记账户信息|本轮材料未展开完整账号|本轮未展开的登记账户信息/gu, "需结合开户资料逐号补齐登记账户信息"],
  [/本轮不宜/gu, "现阶段不宜"],
  [/本轮材料/gu, "现有材料"],
  [/当前摘要未逐笔列明|当前材料未逐笔列明/gu, "当前材料尚未固定逐笔明细"],
  [/[^。；\n]{0,8}未逐笔展开[^。；\n]{0,24}/gu, "需结合回单、流水号和账户明细逐笔固定"],
  [/本轮未展开逐笔明细表/gu, "重点交易明细需逐笔列明"],
  [/未展开逐笔明细表/gu, "重点交易明细需逐笔列明"],
  [/未展开\s*(\d+\s*笔)?([^。；\n]{0,24})/gu, (_match, count = "", tail = "") => {
    const subject = `${count}${tail}`.trim();
    return subject ? `相关${subject}需在附件中逐笔列明` : "相关内容需在附件中逐项列明";
  }],
  [/完整名下账户数仍需以开户资料[/／、和及主体账户清单]*补证|完整名下账户数仍需[^。；\n]{0,30}补证/gu, "本次转账涉及账户已在上表列明；完整开户清单另作主体账户清单核验"],
  [/若需制作附件[，,、]?\s*应导出/gu, "在已调取流水中导出"],
  [/全期间同名(?:对手户名|收款人)?聚合(?:口径)?|同名对手全期间聚合|全期间同名收款人统计/gu, "同名收款人核验结果"],
  [/全期间扣除该集中(?:链路|转账)后|扣除该集中(?:链路|转账)后/gu, "集中交易之外的逐笔往来"],
  [/全区间|全期间(?:汇总金额|转账|往来)?/gu, "统计期间内"],
  [/多账户|多个账户/gu, "多张付款/收款账户"],
  [/完整往来|历史往来/gu, "长期转账关系"],
  [/集中链路/gu, "集中交易"],
  [/零散历史往来/gu, "集中交易之外的逐笔往来"],
  [/需结合用途材料复核/gu, "摘要/类型提示用途线索，需先核对流水备注、交易类型、回单和双方说明"],
  [/必须保留/gu, "需在材料中列明"],
  [/不要合并或改名/gu, "不得混同"],
  [/(?:建议)?导出核对该\s*\d+\s*笔明细/gu, "已返回的小表应先逐笔列明；导出用于附件制作和字段复核"],
  [/调取[^。；\n]{0,40}双方完整流水|调取双方完整流水|调取完整流水|调取[^。；\n]{0,24}全量流水/gu, "在已调取流水中核对缺失字段，必要时补取外部材料"],
  [/建议重点核对[:：]?[^\n。；]{0,60}后续(?:出账|转出)明细|需(?:补取|调取)[^\n。；]{0,24}收款账户后续(?:出账|转出)明细|继续(?:研判|追查|调取)[^\n。；]{0,24}收款账户后续(?:出账|转出)明细/gu, "先用当前案件已导入的收款方流水核验承接/分流，再补取外部凭证证明资金对应关系"],
  [/案内账户存在[，,；;]?\s*但本窗口未见继续出账/gu, "账号已在本案账户范围出现；已调取流水未匹配到该账号接续出账，列为继续穿透对象"],
  [/本次核验窗口内未匹配到后续出账|本窗口未见继续出账|核验窗口内未匹配到后续出账/gu, "已调取流水范围内未匹配到接续出账"],
  [/本次核验窗口|核验窗口|本窗口/gu, "已调取流水范围"],
  [/\bsupported\s+seeds?\b/giu, "本轮可核验交易"],
  [/\bCandidate\s+Edges?\b/giu, "待复核链路"],
  [/\bConfirmed\s+Edges?\b/giu, "已有流水支持的链路"],
  [/候选边[^。；\n]{0,12}确认/gu, "相关链路需继续补证固定"],
  [/候选边/gu, "待复核链路"],
  [/\bsupported\s+transaction\s+edges?\b|\bsupported\s+edges?\b/giu, "可证实资金链路"],
  [/\bunsupported\s+flows?\b/giu, "证据不足的资金流"],
  [/\bunsupported\b/giu, "证据不足"],
  [/\bsupported\b/giu, "已有数据支持"],
  [/\bsupport[-_ ]layer\b/giu, "支撑依据"],
  [/\bsupport\b/giu, "支持依据"],
  [/\bneeds[_ -]?review\b|\bneeds\s+data\b/giu, "需补证"],
  [/\bpartial\b/giu, "部分可见/需补证"],
  [/\bcandidate[-_ ]?only\b/giu, "仅作线索"],
  [/\bcandidates?\b/giu, "线索"],
  [/\bblocked\b|\bblock\b/giu, "当前证据不足"],
  [/\bpass\b/giu, "核验通过"],
  [/\bdowngraded\b|\bdowngrade\b/giu, "降级为线索"],
	  [/\bvalidation[_ ]state\b/giu, "核验状态"],
	  [/\bdelivery[_ ]contract\b/giu, "交付要求"],
	  [/\bdelivery[_ ]state\b/giu, "交付状态"],
	  [/\bsource[-_ ]of[-_ ]truth\b|\bsource\s+of\s+truth\b/giu, "权威来源"],
	  [/\bexplicit_case_lookup\b/giu, "显式案件编号核验"],
	  [/\bbackend_active_case\b|\bbackend\s+active[-_ ]case\b|\bactive[-_ ]case\b/giu, "当前案件确认"],
	  [/\bfailed\s+after\s+transient\s+retry\b/giu, "重试后仍未完成"],
	  [/\bsemantic\s+MCP\s+tool\b/giu, "案件事实核验环节"],
	  [/\bruntime\s+context\b/giu, "核验上下文"],
	  [/\bsource[_ ]envelope\b/giu, "本次依据与统计范围"],
  [/\bsource[_ ]boundary\b/giu, "本次依据与统计范围"],
  [/\bartifacts?\b/giu, "交付材料"],
  [/\bruntime\b/giu, "插件运行环境"],
  [/\bclaim[-_ ]?review\b/giu, "报告事实结论复核"],
  [/\bclaims?\b/giu, "研判结论"],
  [/\bevidence[_ ]ledger\b/giu, "证据记录"],
  [/\bgraph\s+nodes?\b/giu, "图谱节点"],
  [/\bgraph\s+edges?\b/giu, "图谱链路"],
  [/\braw\s+rows?\b/giu, "明细行"],
  [/\bJSON\b/gu, "结构化内容"],
  [/\bdebug\b/giu, ""],
  [/\bfollow[- ]up\b/giu, "下一步追查"],
  [/\bcasegraph\b/giu, "案件关系图"],
  [/\bfundgraph\b/giu, "资金关系图"],
  [/\bgraph[-_ ]?visualization\b/giu, "资金流向图谱核验"],
  [/\bVisual\s+Evidence\b/gu, "可视化证据"],
  [/\banswer[_ ]card\b/giu, "研判卡"],
  [/\bready_from_rank_facts\b/giu, "已具备排行事实"],
  [/\bready_from_holder_facts\b/giu, "已具备户名事实"],
  [/\bscope_edges_ready_trace_edges_need_flow_tool\b/giu, "范围线索已具备，资金链路需补资金穿透核验"],
  [/\bready_as_review_risk\b/giu, "已作为复核风险"],
  [/\bready_from_gates\b/giu, "已具备质量门结果"],
  [/\broadmap_ready_probe_required\b/giu, "路线已具备，需补线索检验"],
  [/\broadmap_ready_source_gap_required\b/giu, "路线已具备，需补来源材料"],
  [/\bwrite[_ ]blocked\b/giu, "暂不出具正式结论"],
  [/\breport\s+gate\b/giu, "报告级复核"],
  [/\bdiagnostic\s+labels?\b/giu, "复核标签"],
  [/\bdoctor\b/giu, "健康检查"],
  [/\bscore\b/giu, "评分"],
  [/\bfunds_investigate\b/giu, "资金研判入口"],
  [/\bworkflow\b/giu, "研判流程"],
  [/\brank_counterparties\b/giu, "对手方排行核验"],
  [/\brank_accounts\b/giu, "账户排行核验"],
  [/\brank_holders\b/giu, "主体排行核验"],
  [/\btrace_fund_next_hop\b|\btrace_fund\b/giu, "资金追踪"],
  [/\btrace_subject_top_outflows\b/giu, "主体出账去向核验"],
  [/\bbuild_fund_flow_graph\b/giu, "资金穿透图核验"],
  [/\bvalidate_report_claims\b/giu, "报告事实结论复核"],
  [/\bvalidate_continuation_list\b/giu, "续查清单复核"],
  [/\brun_full_case_analysis\b/giu, "全案研判"],
  [/\bcompare_analysis_scopes\b/giu, "对比核验"],
  [/\bhypothesis_probe\b/giu, "线索检验"],
	  [/\bdata_quality\b/giu, "数据质量核验"],
	  [/\bpipeline\b/giu, "数据处理环节"],
	  [/\btimed\s+out\s+after\s+\d+\s*ms\b/giu, "未在限定时间内完成"],
	  [/__unknown__|counterparty:unknown/giu, "对手字段缺失"],
	  [/\baudit_case_data_quality\b/giu, "数据质量与口径复核"],
  [/\bget_case_scope_map\b/giu, "案件数据范围核验"],
  [/\bget_current_case\b/giu, "当前案件确认"],
  [/\banalysis_txn_detail_idx\b/giu, "清洗/分析索引"],
  [/\banalysis_[a-z0-9_]+\b/giu, "清洗/分析索引"],
  [/\bfc_[a-z0-9_]*_norm\b/giu, "清洗后的业务表"],
  [/\bedge_status\b/giu, "链路状态"],
  [/\bfact_refs\b/giu, "事实依据"],
  [/\bcounterparty_name\b/giu, "对手方名称"],
  [/\bcounterparty_acct_norm\b/giu, "对手方账号"],
  [/\bcp_key\b/giu, "对手方识别键"],
  [/\bturnover\b/giu, "往来总额"],
  [/\bCASE_SOURCE_BLOCKER\b|\bINVALID_CASE_SOURCE\b/gu, "当前案件来源未就绪"],
  [/\bmini_review_ready\b/giu, "小额核验已形成"],
  [/\bbounded_workbench_executed\b/giu, "专项资金核算已执行"],
  [/\bworkbench_gap\b/giu, "专项资金核算条件不足"],
  [/\branking_only\b/giu, "仅作排行线索"],
  [/\bholder_analysis\b/giu, "主体研判"],
  [/\branking\b/giu, "排行"],
  [/\bsource-backed\b/giu, "有来源支撑"],
  [/\btargeted\s+verification\b/giu, "定向核验"],
  [/\btargeted\b/giu, "定向"],
  [/\baccounts\b/giu, "账户"],
  [/\bholders\b/giu, "主体"],
  [/\bcounterparties\b/giu, "对手方"],
  [/\bPair\s+Amount\b/giu, "金额核验"],
  [/\bAmount\s+Challenge\b/giu, "金额争议核验"],
  [/\bMini[- ]Review\b/giu, "简要核验材料"],
  [/\brank\s+facts\b/giu, "排行事实"],
  [/\btrace\s+facts\b/giu, "追踪事实"],
  [/\bfacts\b/giu, "事实"],
  [/\bfact\b/giu, "事实"],
  [/\bContext\s+must\s+write\b/giu, "研判依据"],
  [/\bclaim_(\d+)\b/giu, "第$1项"],
  [/\bstatus\s*=\s*/giu, "核验状态："],
  [/\bok\b/giu, "已完成"],
  [/\bdossier\b/giu, "证据包"],
  [/\btracing\b/giu, "资金追踪"],
  [/\braw\b/giu, "原明细"],
  [/\beffective\b/giu, "去重后金额"],
  [/\bonly\b/giu, "仅"],
  [/\bcannot\s+be\s+final\b/giu, "不能作为最终结论"],
  [/\boutput\b/giu, "输出"],
  [/\brank_\*/giu, "排行核验"],
  [/\bcoverage\b/giu, "覆盖范围"],
  [/\bbefore\b/giu, "前置"],
  [/\bwithout\b/giu, "缺少"],
  [/\bwith\b/giu, "含"],
  [/\barrow\b/giu, "箭头"],
  [/\bcard\b/giu, "卡片"],
  [/\bare\b/giu, "已"],
  [/\bdedup\b/giu, "去重"],
  [/\braw\s+detail\b/giu, "原明细"],
  [/\btxn_id\b/giu, "交易号"],
	  [/\bduplicate\/unsupported\b/giu, "重复或证据不足"],
	  [/\bduplicate_families\b/giu, "重复/换卡复核"],
	  [/\bsame_fact\b/giu, "同事实复核"],
	  [/\bduplicate\b/giu, "重复"],
  [/\bhigh-confidence\s+same-fact\b/giu, "高置信同事实"],
  [/\bhigh-confidence\b/giu, "高置信"],
  [/\bendpoint-complete\b/giu, "端点完整"],
  [/\bfull[-_ ]case\b/giu, "全案"],
  [/\bnotebook\b/giu, "可回放核算记录"],
  [/\bbackend\b/giu, "数据服务"],
  [/\bdashboard\b/giu, "看板"],
  [/\bappendix\b/giu, "附件"],
  [/\bworkbook\b/giu, "工作簿"],
  [/\binventory\b/giu, "清单"],
  [/\bfeature\s+family\b/giu, "特征族"],
  [/\blead\b/giu, "线索"],
  [/\blane\b/giu, "分析路径"],
  [/\bevidence\s+pack\b/giu, "证据包"],
  [/\bdeterministic\s+facts?\b/giu, "确定性事实"],
  [/\bresults?\b/giu, "核验结果"],
  [/\bboundary\b/giu, "边界"],
  [/\bforbidden\b/giu, "禁用表述"],
  [/\bcorrected\b/giu, "需纠正"],
  [/\bverified\b/giu, "已核验"],
  [/\bcontinuation\b/giu, "续查"],
  [/\brescope\b/giu, "重算范围"],
  [/\bhypothesis\b/giu, "假设"],
  [/\bmissing\b/giu, "缺失"],
  [/\brows_total\b/giu, "原始总行数"],
  [/\bcase\/task\/intent\b/giu, "案件/任务/意图"],
  [/\bdiscovery\b/giu, "扩展发现"],
  [/\brank\/probe\b/giu, "排行/线索检验"],
  [/\btrace\/graph\b/giu, "追踪/图谱"],
  [/\bunavailable\b/giu, "暂不可用"],
  [/\bstatus\b/giu, "状态"],
  [/\bsource\b/giu, "来源"],
  [/\bscope\b/giu, "范围"],
  [/\bmetric\b/giu, "统计指标"],
  [/\bunit\b/giu, "单位"],
  [/\btime\s+window\b/giu, "时间范围"],
  [/\bevidence\s+status\b/giu, "支持程度"],
  [/\btool\b/giu, "核验环节"],
  [/\bskill\b/giu, "专项能力"],
  [/\bMCP\b/gu, "案件事实工具"],
  [/\bSQL\b/gu, "专项资金核算"],
  [/\bAPI\b/gu, "服务接口"],
  [/执行器/gu, "核验环节"],
  [/查询执行/gu, "核验"],
  [/表结构/gu, "字段范围"],
  [/语义层/gu, "已审计事实"],
  [/来源边界/gu, "本次依据与统计范围"],
  [/(?:^|\n)\s*(?:结论先说|直接说结论)[:：]?\s*/gu, "\n"],
  [/值得注意的是[，,]?\s*/gu, ""],
  [/不难发现[，,]?\s*/gu, "经梳理，"],
  [/综上所述[，,]?\s*|总的来说[，,]?\s*|总体来看[，,]?\s*|归根结底[，,]?\s*|最终来看[，,]?\s*/gu, "经梳理，"],
  [/下面我(?:会|将)[^。；\n]{0,80}(?:[。；]|$)/gu, ""],
  [/让我们(?:一起)?[^。；\n]{0,40}(?:看|分析|梳理|展开)[^。；\n]*(?:[。；]|$)/gu, ""],
  [/接下来(?:我|我们)?(?:会|将)?[^。；\n]{0,60}(?:[。；]|$)/gu, ""],
  [/希望这对你有帮助|希望以上(?:内容|分析)[^。；\n]{0,20}(?:有帮助|可供参考)/gu, ""],
  [/(?:如需|如果需要|需要的话)[^。；\n]{0,40}(?:我可以|可继续|可以继续)[^。；\n]*(?:[。；]|$)/gu, ""],
  [/具有重要意义/gu, "对固定资金关系有意义"],
  [/意义重大/gu, "需结合具体流水说明案件意义"],
  [/提供(?:了)?有力支撑|有力支撑/gu, "可作为后续核查依据"],
  [/奠定(?:了)?基础/gu, "可作为后续核查起点"],
  [/形成(?:了)?闭环/gu, "形成可复核链条"],
  [/赋能/gu, "支持"],
  [/持续优化/gu, "继续核查"],
  [/全面提升/gu, "提高"],
  [/精准识别/gu, "识别"],
  [/深刻揭示|生动体现/gu, "提示"],
  [/底层逻辑/gu, "资金关系"],
  [/核心密码/gu, "关键线索"],
  [/关键抓手|重要抓手/gu, "重点核查方向"],
  [/保驾护航/gu, "支持核查"],
  [/降本增效|提质增效/gu, "提高核查效率"],
  [/多维度赋能|多维度/gu, "多项"],
  [/全链路闭环|全链路/gu, "资金链路"],
  [/体系化推进/gu, "按步骤核查"],
  [/高度重视/gu, "需围绕证据核查"],
  [/压实责任/gu, "明确补证责任"],
  [/形成合力/gu, "协同核查"],
  [/纵深推进/gu, "继续核查"],
  [/取得实效/gu, "形成可复核结果"],
  [/坚实基础|强力支撑/gu, "核查依据"],
  [/违法所得|赃款|非法所得/gu, "资金性质待补证"],
  [/洗钱事实成立|虚开事实成立/gu, "相关法律评价需另行结合证据判断"],
  [/犯罪团伙|共同犯罪|跑分团伙/gu, "人员/账户协同线索"],
  [/(?:已|已经|可|可以|能够|足以)(?:认定|证明|证实)[^。；\n]{0,16}(?:实际控制|代持|最终归属)/gu, "相关控制/归属关系需补证固定"],
  [/(?<!不能)(?<!未能)(?:证明|证实)[^。；\n]{0,16}(?:实际控制|代持|最终归属)/gu, "相关控制/归属关系需补证固定"],
  [/(?:实际控制|代持|最终归属)[^。；\n]{0,12}(?:坐实|锁定|已认定|已经认定)/gu, "相关控制/归属关系需补证固定"],
  [/(?:已|已经)?(?:坐实|锁定)/gu, "需结合证据固定"],
  [/簇/gu, "集合"]
];

export const USER_VISIBLE_FORBIDDEN_PATTERNS = [
  ["quick-fact", /\bquick-fact\b/iu],
  ["SKILL.md", /\bSKILL\.md\b/iu],
  ["List MCP resources", /\bList\s+MCP\s+resources\b/iu],
  ["MCP resources", /\bMCP\s+resources\b/iu],
  ["Shell", /\bShell\b/u],
  ["Codex", /\bCodex\b/u],
  ["local path", /(?:\/Users|\/var)\/[^\s"'，。；、)）]+/u],
  ["absolute artifact path", /(?:\/Users|\/var)\/[^\s"'，。；、)）]*\.(?:docx|xlsx|png|jpg|jpeg|pdf|txt)\b/u],
  ["shell command", /\b(?:cat|grep|find|ls)\s+\/Users\//iu],
  ["mcp__", /\bmcp__[\w.-]+\b/iu],
  ["run_case_sql", /\brun_case_sql\b/iu],
  ["export_cleaned_case_data", /\bexport_cleaned_case_data\b/iu],
  ["Workbench", /\b(?:Controlled\s+Case\s+Workbench|Controlled\s+Workbench|Case\s+Workbench|Workbench)\b/iu],
  ["DuckDB", /\bDuckDB\b/iu],
  ["case_id", /\bcase_id\b/iu],
  ["query_id", /\bquery_ids?\b/iu],
  ["source_hash", /\bsource_hash\b/iu],
  ["metric_scope", /\bmetric_scope\b/iu],
  ["raw_rows_exposed", /\braw_rows_exposed\b/iu],
  ["current_case_only", /\bcurrent_case_only\b/iu],
  ["cleaned_analysis_scope_only", /\bcleaned_analysis_scope_only\b/iu],
  ["parser_binder_validated", /\bparser_binder_validated\b/iu],
  ["row_limit", /\brow_limit\b/iu],
  ["case id value", /(?:案件(?:编号|ID)|\bcase[_\s-]*id)\s*[:：]?\s*[0-9a-f][0-9a-f-]{7,}\b/iu],
  ["workflow", /\bworkflow\b/iu],
  ["support layer", /\bsupport[-_ ]layer\b/iu],
  ["裸账户组表述", /账户组/u],
  ["机械收款端表述", /收款端|主体账户集合/u],
  ["机械命中表述", /本轮命中/u],
  ["机械同事实表述", /同事实去重(?:后(?:的)?金额|代表|支持|后)?|高可信追踪种子|已有规范明细(?:\/分析图)?支持|证据状态/u],
  ["证据边界", /证据边界/u],
  ["机械明细表述", /原始命中记录|原命中明细|原始命中明细|原始匹配明细|原始匹配记录|命中明细|命中记录|本轮已返回|本轮未展开逐笔明细表|未展开逐笔明细表|本轮未展开|未展开(?:\s*\d+\s*笔)?|未逐笔展开|未回传|未返回逐笔字段/u],
  ["轮次材料表述", /本轮材料|本轮不宜|补齐本轮未展开/u],
  ["内部摘要表述", /当前事实摘要|当前摘要|当前摘录/u],
  ["非标准进度句", /正在(?!核验当前案件事实。)[^。\n；]{0,40}核验/u],
  ["笼统范围表述", /全区间|全期间(?:汇总金额|转账|往来)?|多账户|多个账户|完整往来|历史往来|集中链路/u],
  ["折叠明细行表述", /其余\s*(?:\d+\s*笔|数笔|链路|后续链路|去向)|多个账户\s*\|\s*多个账户|合计\s*[\d,.]+\s*(?:元)?\s*\|\s*小额转账/u],
  ["机械账户补证套话", /完整名下账户数仍需以开户资料[/／、和及主体账户清单]*补证|完整名下账户数仍需[^。；\n]{0,30}补证/u],
  ["机械附件制作套话", /若需制作附件[，,、]?\s*应导出/u],
  ["残差桶表述", /全期间扣除该集中(?:链路|转账)后|扣除该集中(?:链路|转账)后|零散历史往来|需结合用途材料复核/u],
  ["完整小表未展开", /(?:建议)?导出核对该\s*\d+\s*笔明细/u],
  ["重复调取流水建议", /调取[^。；\n]{0,40}双方完整流水|调取双方完整流水|调取完整流水|调取[^。；\n]{0,24}全量流水/u],
  ["已见后续出账仍泛化取证", /建议重点核对[:：]?[^\n。；]{0,60}后续(?:出账|转出)明细|需(?:补取|调取)[^\n。；]{0,24}收款账户后续(?:出账|转出)明细|继续(?:研判|追查|调取)[^\n。；]{0,24}收款账户后续(?:出账|转出)明细/u],
  ["窗口化追踪表述", /本窗口|核验窗口|本次核验窗口/u],
  ["Mermaid", /\bMermaid\b/iu],
  ["英文图谱状态词", /\b(?:supported|needs[_ -]?review|candidate|edge_status|delivery_state)\b/iu],
  ["candidate/confirmed edge", /\b(?:candidate|confirmed)\s+edges?\b|候选边[^。；\n]{0,12}确认/iu],
  ["经侦禁用术语-簇", /簇/u],
  ["cluster", /\bclusters?\b|\bfocus[_ -]?cluster\b|\boutside[_ -]?focus[_ -]?cluster\b/iu],
  ["supported", /\bsupported\b/iu],
  ["unsupported", /\bunsupported\b/iu],
  ["needs_review", /\bneeds[_ -]?review\b/iu],
  ["candidate", /\bcandidate\b/iu],
  ["edge_status", /\bedge_status\b/iu],
  ["delivery_state", /\bdelivery_state\b/iu],
  ["blocked", /\bblocked\b/iu],
  ["block", /\bblock\b/iu],
  ["pass", /\bpass\b/iu],
  ["partial", /\bpartial\b/iu],
  ["status", /\bstatus\b/iu],
  ["claim", /\bclaim\b/iu],
  ["MCP", /\bMCP\b/u],
  ["SQL", /\bSQL\b/u],
  ["执行器", /执行器/u],
  ["专项明细查询", /专项\s*(?:SQL|明细查询)|明细字段与交易行/u],
  ["tool", /\btool\b/iu],
  ["skill", /\bskill\b/iu],
  ["Pair Amount", /\bPair\s+Amount\b/iu],
  ["Amount Challenge", /\bAmount\s+Challenge\b/iu],
  ["debug", /\bdebug\b/iu],
  ["debug-cn", /排障信息/u],
  ["JSON", /\bJSON\b/u],
  ["pipeline", /\bpipeline\b/iu],
  ["data_quality", /\bdata_quality\b/iu],
  ["timed out", /\btimed\s+out\s+after\s+\d+\s*ms\b/iu],
  ["unknown placeholder", /__unknown__|counterparty:unknown/iu],
  ["raw", /\braw\b/iu],
  ["raw rows", /\braw\s+rows?\b/iu],
  ["runtime", /\bruntime\b/iu],
	  ["source envelope", /\bsource[_ ]envelope\b/iu],
	  ["source-of-truth", /\bsource[-_ ]of[-_ ]truth\b|\bsource\s+of\s+truth\b/iu],
	  ["explicit_case_lookup", /\bexplicit_case_lookup\b/iu],
	  ["case project", /\bbackend_active_case\b|\bbackend\s+active[- ]case\b|\bactive[- ]case\b/iu],
	  ["failed after transient retry", /\bfailed\s+after\s+transient\s+retry\b/iu],
	  ["semantic MCP tool", /\bsemantic\s+MCP\s+tool\b/iu],
	  ["runtime context", /\bruntime\s+context\b/iu],
  ["validation state", /\bvalidation[_ ]state\b/iu],
  ["delivery contract", /\bdelivery[_ ]contract\b/iu],
  ["artifact", /\bartifacts?\b/iu],
  ["evidence ledger", /\bevidence[_ ]ledger\b/iu],
  ["diagnostic label", /\bdiagnostic\s+labels?\b/iu],
  ["doctor", /\bdoctor\b/iu],
  ["score", /\bscore\b/iu],
  ["internal id", /\binternal\s+id\b/iu],
  ["write_blocked", /\bwrite[_ ]blocked\b/iu],
  ["Context must write", /\bContext\s+must\s+write\b/iu],
  ["机械开头", /按当前案件、未限定时间和账户口径核验/u],
  ["计划式进度泄漏", /(?:^|\n)\s*(?:我先|我会|我将)[^。\n；]{0,80}(?:核验|读取|整理|流程|材料|交易明细|口径)/u],
  ["双方资金往来核验流程公告泄漏", /我会使用[“"`']?pair-amount-investigation[”"`']?资金往来核验流程|(?:我会|我将)使用[“"`']?(?:两方金额核验|特定双方资金往来核验)[”"`']?(?:技能|流程)?|(?:我会|我将)使用[^。\n；]{0,40}pair-amount-investigation|(?:我会|我将)使用[^。\n；]{0,40}Pair\s+Amount/iu],
  ["先核对统计口径泄漏", /先核对当前案件数据中[^。\n；]{0,60}统计口径|再给出金额结论和证据边界/u],
  ["进度口径泄漏", /(?:我会|将|正在|先)?[^。\n；]{0,20}按[^。\n；]{0,40}口径/u],
  ["读取技能说明泄漏", /(?:先)?读取[^。\n；]{0,20}技能说明|技能文件|先看[^。\n；]{0,20}文件|读取案件数据口径|可用交易明细/u],
  ["内部金额统计标题", /全期间同名对手户名聚合(?:口径)?|同名对手全期间聚合|核心收款账号(?:确定)?口径|按核心收款账号核验/u],
  ["旧金额核验标题", /金额核验\s*小研判/u],
  ["旧金额口径标题", /金额口径|口径差异说明|口径提示/u],
  ["旧收款账号统计标题", /重点收款账号统计/u],
  ["旧重点收款账户核验标题", /重点收款账户核验/u],
  ["旧重点收款账户情况标题", /重点收款账户情况/u],
  ["旧金额核算说明标题", /金额核算说明/u],
  ["旧同名匹配结果标题", /同名收款人匹配结果/u],
  ["旧当前已调取数据标题", /当前已调取数据中/u],
  ["当前案件可见", /当前案件可见/u],
  ["来源边界", /来源边界/u],
  ["AI套话开场", /结论先说|直接说结论|值得注意的是|不难发现/u],
  ["AI套话收尾", /综上所述|总的来说|总体来看|归根结底|最终来看/u],
  ["AI二元拔高", /不(?:只|仅)(?:是|仅是)[^。；\n]{0,40}(?:更是|而是)|不仅[^。；\n]{0,40}更/u],
  ["AI三段式机械列举", /首先[^。；\n]{0,80}其次[^。；\n]{0,80}(?:最后|再次|第三)/u],
  ["空泛案件意义", /具有重要意义|意义重大|提供(?:了)?有力支撑|有力支撑|奠定(?:了)?基础|形成(?:了)?闭环|赋能|持续优化|全面提升|精准识别/u],
  ["无源权威铺垫", /(?:业内人士认为|相关专家指出|实践表明|事实证明)[^。；\n]{0,60}/u],
  ["AI元评论尾巴", /下面我会|下面我将|让我们(?:一起)?|接下来(?:我|我们)?(?:会|将)?|希望这对你有帮助|如需[^。；\n]{0,40}(?:我可以|可继续|可以继续)|如果需要[^。；\n]{0,40}(?:我可以|可继续|可以继续)/u],
  ["自媒体洞察腔", /深刻揭示|生动体现|底层逻辑|核心密码|关键抓手|重要抓手|保驾护航|有温度/u],
  ["咨询化泛化", /降本增效|提质增效|多维度赋能|全链路闭环|体系化推进|形成合力/u],
  ["公安公文空话", /高度重视|压实责任|纵深推进|取得实效|坚实基础|强力支撑/u],
  ["法律定性越界", /违法所得|赃款|非法所得|洗钱事实成立|虚开事实成立|犯罪团伙|共同犯罪|跑分团伙|(?:已|已经|可|可以|能够|足以)(?:认定|证明|证实)[^。；\n]{0,16}(?:实际控制|代持|最终归属)|(?<!不能)(?<!未能)(?:证明|证实)[^。；\n]{0,16}(?:实际控制|代持|最终归属)|(?:坐实|锁定)[^。；\n]{0,16}(?:实际控制|代持|最终归属)|(?:实际控制|代持|最终归属)[^。；\n]{0,12}(?:坐实|锁定|已认定|已经认定)/u],
  ["事实卡", /事实卡/u],
  ["证据要点", /证据要点/u],
  ["核验摘要", /核验摘要/u],
  ["材料状态", /材料状态/u],
  ["风险/复核卡", /风险\/复核卡/u],
  ["核心收款账号", /核心收款账号/u],
  ["内部图谱标题", /口径与来源边界|数据事实|已有数据支持的资金边（资金流向图依据表）|资金流向图（仅使用上表已有数据支持的资金边）/u],
  ["审计层资金边表述", /已有数据支持的(?:交易级)?资金边/u],
  ["资金边", /资金边/u],
  ["指令型报告骨架", /作答骨架|第一行保留/u],
  ["Subject Dossier", /\bSubject\s+Dossier\b/u],
  ["direct/candidate", /归属边界（direct\s*\/\s*candidate）/u],
  ["候选账户零值标题", /候选线索账户\s*0\s*个/u],
  ["指令型标题合同", /必须保留|不要合并或改名/u],
  ["claim id", /\bclaim_\d+\b/iu],
  ["targeted", /\btargeted\b/iu],
  ["fact_refs", /\bfact_refs\b/iu],
  ["raw field", /\b(?:counterparty_name|counterparty_acct_norm|cp_key|analysis_[a-z0-9_]+|fc_[a-z0-9_]*_norm)\b/iu],
  ["Mini-Review", /\bMini[- ]Review\b/iu],
	  ["duplicate", /\bduplicate\b/iu],
	  ["duplicate_families", /\bduplicate_families\b/iu],
	  ["same_fact", /\bsame_fact\b/iu],
  ["support", /\bsupport\b/iu],
  ["ready state", /\b(?:ready_from_rank_facts|ready_from_holder_facts|scope_edges_ready_trace_edges_need_flow_tool|ready_as_review_risk|ready_from_gates|roadmap_ready_probe_required|roadmap_ready_source_gap_required)\b/iu],
  ["evidence pack", /\bevidence\s+pack\b/iu],
  ["deterministic fact", /\bdeterministic\s+facts?\b/iu],
  ["deterministic", /\bdeterministic\b/iu],
  ["facts", /\bfacts?\b/iu],
  ["boundary", /\bboundary\b/iu],
  ["forbidden", /\bforbidden\b/iu],
  ["verified", /\bverified\b/iu],
  ["corrected", /\bcorrected\b/iu],
  ["result", /\bresults?\b/iu],
  ["continuation", /\bcontinuation\b/iu],
  ["rescope", /\brescope\b/iu],
  ["hypothesis", /\bhypothesis\b/iu],
  ["discovery", /\bdiscovery\b/iu],
  ["case/task/intent", /\bcase\/task\/intent\b/iu],
  ["rank/probe", /\brank\/probe\b/iu],
  ["trace/graph", /\btrace\/graph\b/iu],
  ["accounts", /\baccounts\b/iu],
  ["holder_analysis", /\bholder_analysis\b/iu],
  ["ranking", /\branking\b/iu],
  ["high-confidence", /\bhigh-confidence\b/iu],
  ["tool-cn", /工具/u],
  ["unverified-final-cn", /核准无误/u]
];

export function translateUserVisibleText(value) {
  let output = text(value);
  for (const [pattern, replacement] of TERM_REPLACEMENTS) {
    output = output.replace(pattern, replacement);
  }
  return output
    .replace(/本工具可支持/gu, "本次核验可支持")
    .replace(/本工具/gu, "本次核验")
    .replace(/工具复核/gu, "核验环节复核")
    .replace(/工具/gu, "核验环节")
    .replace(/执行器/gu, "核验环节")
    .replace(/查询执行/gu, "核验")
    .replace(/表结构/gu, "字段范围")
    .replace(/语义层/gu, "已审计事实")
    .replace(/模型应/gu, "应")
    .replace(/模型侧汇总/gu, "非来源汇总")
    .replace(/前门/gu, "入口")
    .replace(/金额核验\s*小研判(?:\s*\/\s*金额核验\s*小研判)?/gu, "经梳理")
    .replace(/集中转账簇|集中交易簇|集中簇/gu, "集中交易组")
    .replace(/其余链路|其余后续链路|其余去向/gu, "尚未固定的后续去向")
    .replace(/其余\s*(\d+\s*笔)/gu, "集中交易以外 $1")
    .replace(/其余数笔/gu, "尚未固定的数笔")
    .replace(/其余账号/gu, "尚未列明的账号")
    .replace(/簇外\s*(\d+\s*笔)/gu, "集中交易以外 $1")
    .replace(/簇外往来|簇外行/gu, "集中交易以外逐笔往来")
    .replace(/可疑簇/gu, "可疑线索集合")
    .replace(/时间簇/gu, "时间集中")
    .replace(/金额簇/gu, "金额集中")
    .replace(/账户簇/gu, "相关账户（列明账号）")
    .replace(/交易簇/gu, "交易组")
    .replace(/资金簇/gu, "资金集中流向")
    .replace(/簇/gu, "集合")
    .replace(/(?:^|\n)\s*(?:我先|我会|我将)[^。\n；]{0,80}(?:核验|读取|整理|流程|材料|交易明细|口径)[^。\n；]*(?:[。；]|$)/gu, "正在核验当前案件事实。")
    .replace(/(?:我会|将|正在|先)[^。\n；]{0,20}按[^。\n；]{0,40}口径[^。\n；]*(?:[。；]|$)/gu, "正在核验当前案件事实。")
    .replace(/(?:先)?读取[^。\n；]{0,20}技能说明[^。\n；]*(?:[。；]|$)/gu, "正在核验当前案件事实。")
    .replace(/按([^。\n；]{0,40})口径/gu, "按$1统计范围")
    .replace(/核验口径/gu, "核验范围")
    .replace(/金额口径/gu, "金额核验情况")
    .replace(/口径差异说明/gu, "竞争金额业务解释")
    .replace(/口径提示/gu, "补证方向")
    .replace(/全期间扣除该集中(?:链路|转账)后|扣除该集中(?:链路|转账)后/gu, "集中交易之外的逐笔往来")
    .replace(/零散历史往来/gu, "集中交易之外的逐笔往来")
    .replace(/需结合用途材料复核/gu, "摘要/类型提示用途线索，需先核对流水备注、交易类型、回单和双方说明")
    .replace(/必须保留/gu, "需在材料中列明")
    .replace(/不要合并或改名/gu, "不得混同")
    .replace(/同事实去重(?:后(?:的)?金额|代表|支持|后)?/gu, "重复风险复核")
    .replace(/证据状态/gu, "核验意见")
    .replace(/证据边界/gu, "核验意见")
    .replace(/当前事实摘要未回传|当前摘要未回传|事实摘要未回传|摘要未回传|未回传/gu, "当前材料尚未取得")
    .replace(/当前事实摘要|当前摘要|当前摘录/gu, "当前材料")
    .replace(/本轮材料已(?:明确)?展示/gu, "现有材料已列明")
    .replace(/本轮已核到/gu, "现有材料已核实")
    .replace(/补齐本轮未展开的登记账户信息|本轮材料未展开完整账号|本轮未展开的登记账户信息/gu, "需结合开户资料逐号补齐登记账户信息")
    .replace(/本轮不宜/gu, "现阶段不宜")
    .replace(/本轮材料/gu, "现有材料")
    .replace(/当前摘要未逐笔列明|当前材料未逐笔列明/gu, "当前材料尚未固定逐笔明细")
    .replace(/[^。；\n]{0,8}未逐笔展开[^。；\n]{0,24}/gu, "需结合回单、流水号和账户明细逐笔固定")
    .replace(/未展开\s*(\d+\s*笔)?([^。；\n]{0,24})/gu, (_match, count = "", tail = "") => {
      const subject = `${count}${tail}`.trim();
      return subject ? `相关${subject}需在附件中逐笔列明` : "相关内容需在附件中逐项列明";
    })
    .replace(/全期间同名(?:对手户名|收款人)?聚合(?:口径)?|同名对手全期间聚合|全期间同名收款人统计/gu, "同名收款人核验结果")
    .replace(/全期间/gu, "统计期间内")
    .replace(/全区间/gu, "统计期间内")
    .replace(/重点收款账号统计/gu, "重点收款账户流水")
    .replace(/当前案件可见/gu, "已调取流水显示")
    .replace(/当前可见/gu, "已调取流水显示")
    .replace(/当前案件已见/gu, "已调取流水显示")
    .replace(/当前案件中/gu, "本案")
    .replace(/核心窗口口径/gu, "集中交易窗口")
    .replace(/按核心收款账号核验|核心收款账号核验/gu, "重点收款账户流水")
    .replace(/核心收款账号/gu, "重点收款账户")
    .replace(/风险\/复核卡/gu, "风险与复核事项")
    .replace(/材料状态/gu, "材料摘要")
    .replace(/核验摘要/gu, "研判摘要")
    .replace(/证据要点/gu, "研判依据")
    .replace(/必需事实卡/gu, "必需核验事项")
    .replace(/既有报告\s*研判结论\s*复核(?:事实卡|核验材料|材料)/gu, "既有报告研判结论复核材料")
    .replace(/事实卡/gu, "核验材料")
    .replace(/重复\s*\/\s*证据不足/gu, "重复或证据不足")
    .replace(/清洗\/分析索引\s+清洗\/分析索引/gu, "清洗/分析索引")
    .replace(/deterministic\s+事实/giu, "确定性事实")
    .replace(/高置信\s+same-事实/giu, "高置信同事实")
    .replace(/研判结论\s+事实/gu, "研判结论事实")
    .replace(/部分可见\/需补证、缺失、需补证/gu, "部分可见/需补证、缺失")
    .replace(/研判状态:\s*已完成/gu, "研判状态: 已形成可复核摘要")
    .replace(/资金流向图\/路径边界/gu, "资金流向图边界")
    .replace(/交易级可证实资金边/gu, "交易级可证实资金链路")
    .replace(/可证实资金边/gu, "可证实资金链路")
    .replace(/确定性资金边/gu, "确定性资金链路")
    .replace(/资金边/gu, "资金链路")
    .replace(/收款端点/gu, "收款对象")
    .replace(/收款端/gu, "收款对象")
    .replace(/核验通过\s*\/\s*当前证据不足\s*\/\s*降级为线索\s*\/\s*需补证/gu, "核验通过 / 当前证据不足 / 降级为线索 / 需补证")
    .replace(/既有报告\s+研判结论\s+复核材料/gu, "既有报告研判结论复核材料")
    .replace(/研判结论\s+缺少\s+事实依据/gu, "研判结论缺少事实依据")
    .replace(/\s{2,}/gu, " ")
    .replace(/\s+([，。；、,.])/gu, "$1")
    .trim();
}

export function userVisibleLeakageLabels(value) {
  const output = text(value);
  return USER_VISIBLE_FORBIDDEN_PATTERNS
    .filter(([, pattern]) => pattern.test(output))
    .map(([label]) => label);
}

export function assertNoUserVisibleLeakage(value) {
  const labels = userVisibleLeakageLabels(value);
  if (labels.length) {
    throw new Error(`user-visible language leakage: ${labels.join(", ")}`);
  }
}
