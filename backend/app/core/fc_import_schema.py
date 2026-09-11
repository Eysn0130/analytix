from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, List


RAW_SUFFIX = "_raw"
NORM_SUFFIX = "_norm"


def raw_table_name(table: str) -> str:
    return f"{table}{RAW_SUFFIX}"


def norm_table_name(table: str) -> str:
    return f"{table}{NORM_SUFFIX}"


@dataclass
class FcSchema:
    table: str
    headers: List[str]
    col_map: Dict[str, str]
    store_raw_json: bool = True


TXN_HEADER_ALIASES: Dict[str, List[str]] = {
    # Standard -> common variants (keep conservative to avoid mis-maps)
    "交易卡号": ["查询卡号", "本方卡号", "卡号"],
    "交易账号": ["查询账号", "查询帐号", "本方账号", "账号", "账户账号", "账户", "账号/卡号"],
    "账户开户名称": ["姓名", "姓名(查询条件)", "企业名称", "企业名称(查询条件)", "开户名", "户名", "账户名称"],
    "开户人证件号码": ["证件号码", "证件号码(查询条件)", "证件号", "身份证号"],
    "交易时间": ["交易日期", "日期", "发生日期", "交易日期时间", "交易发生时间", "发生时间", "记账日期", "入账时间"],
    "交易金额": ["发生额", "金额", "借方金额", "贷方金额", "收入支出", "收支金额", "收支额", "收支", "交易金额原币", "交易金额原币（元）", "交易金额折人民币", "交易金额折人民币（元）"],
    "交易余额": ["余额", "账户余额", "可用余额"],
    "收付标志": ["借贷标志", "借贷方向", "借方贷方", "进出标志", "借贷标记", "资金收付标识", "资金收付标志", "收付标识"],
    "交易对手账卡号": [
        "交易对方账卡号",
        "交易对方帐卡号",
        "交易对方账号",
        "交易对方卡号",
        "对方账号",
        "对方卡号",
        "对手账号",
        "对手卡号",
    ],
    "对手户名": ["交易对方名称", "交易对方户名", "对方名称", "对手名称"],
    "对手身份证号": ["交易对方证件号码", "交易对方证件号", "对方证件号码", "对方证件号", "对手证件号"],
    "对手开户银行": ["交易对方账号开户行", "交易对方开户行", "交易对方行名称", "对方开户行", "对手开户行", "对方开户银行"],
    "摘要说明": ["交易摘要", "摘要", "交易说明", "交易用途", "用途"],
    "现金标志": ["现金标识", "现金类型", "现金、转账标识", "现金转账标识"],
    "交易币种": ["币种", "币别", "货币", "币种名称"],
    "交易网点名称": ["网点名称", "交易网点", "交易行名称", "交易机构名称"],
    "交易网点代码": ["网点代码", "交易机构代码", "机构代码", "交易机构号", "网点号"],
    "交易是否成功": ["是否成功", "交易结果", "成功标志"],
    "对手交易余额": ["交易对手余额", "对方余额", "对手余额"],
    "交易流水号": ["流水号", "业务流水号"],
    "终端号": ["终端编号", "交易终端号", "设备号", "自助设备编号", "终端代码", "ATM机具编号"],
    "IP地址": ["IP"],
    "MAC地址": ["MAC", "MAC或IMEI地址", "MAC/IMEI地址", "IMEI地址"],
    "凭证种类": ["凭证类型"],
    "凭证号": ["凭证编号"],
    "交易柜员号": ["柜员号"],
    "商户名称": ["商户名", "特约商户名称", "商户户名"],
    "商户号": ["商户编号", "商户代码", "特约商户号", "特约商户编号"],
    "备注": ["交易备注", "备注说明"],
    "交易类型": ["交易种类", "业务类型", "业务种类", "业务名称", "交易类别", "业务类别"],
    "查询反馈结果原因": ["查询反馈结果", "查询结果", "反馈原因"],
}

ACCOUNT_HEADER_ALIASES: Dict[str, List[str]] = {
    "账户开户名称": ["姓名", "姓名(查询条件)", "开户名", "户名", "客户名称", "账户名称"],
    "开户人证件号码": ["证件号码", "证件号码(查询条件)", "证件号", "身份证号"],
    "交易卡号": ["卡号", "本方卡号", "账卡号", "查询卡号"],
    "交易账号": ["账号", "账户账号", "账户号", "本方账号", "本方账户", "查询账号", "查询帐号"],
    "账号开户时间": ["开户日期", "开户时间", "账号开户日期", "开户日期时间"],
    "账户余额": ["余额", "账户余额", "账面余额"],
    "可用余额": ["可用余额", "可用金额", "可用资金"],
    "币种": ["币种", "币别"],
    "开户网点代码": ["开户网点代码", "交易网点代码", "网点代码"],
    "开户网点": ["开户网点", "网点名称", "开户机构"],
    "账户状态": ["账户状态", "账号状态", "账户状态名称"],
    "钞汇标志名称": ["钞汇标志", "钞汇标志名称", "钞汇"],
    "销户日期": ["销户日期", "销户时间"],
    "账户类型": ["账户类型", "账户类别"],
    "备注": ["备注", "附言", "说明"],
    "账号开户银行": ["开户银行", "开户行", "开户行名称", "账号开户行"],
    "销户网点": ["销户网点", "销户机构"],
    "最后交易时间": ["最后交易时间", "最后交易日期"],
}

SUB_ACCOUNT_HEADER_ALIASES: Dict[str, List[str]] = {
    "银行名称": ["开户银行", "开户行", "银行"],
    "开户账号": ["账卡号", "账户账号", "账号", "主账户账号", "本方账号", "卡号"],
    "子账户账号": ["子账户账号", "子账号", "子账户号"],
    "余额": ["账户余额", "子账户余额", "余额"],
    "可用余额": ["可用余额", "可用金额"],
    "子账户类别": ["子账户类别", "子账户类型"],
    "子账户序号": ["子账户序号", "子账户编号", "子账户顺序号"],
    "币种": ["币种", "币别"],
    "钞汇标志": ["钞汇标志", "钞汇标志名称", "钞汇"],
    "账户状态": ["账户状态", "账号状态"],
    "账户序号": ["账户序号", "总账户序号", "主账户序号"],
}

HEADER_ALIASES_BY_TABLE: Dict[str, Dict[str, List[str]]] = {
    "fc_transaction": TXN_HEADER_ALIASES,
    "fc_account": ACCOUNT_HEADER_ALIASES,
    "fc_sub_account": SUB_ACCOUNT_HEADER_ALIASES,
}


FC_SCHEMAS: Dict[str, FcSchema] = {
    "fc_account": FcSchema(
        table="fc_account",
        headers=[
            "账户开户名称","开户人证件号码","交易卡号","交易账号","账号开户时间","账户余额","可用余额","币种",
            "开户网点代码","开户网点","账户状态","钞汇标志名称","销户日期","账户类型","备注","账号开户银行",
            "销户网点","最后交易时间"
        ],
        col_map={
            "账户开户名称":"account_open_name",
            "开户人证件号码":"opener_id_no",
            "交易卡号":"card_no",
            "交易账号":"acct_no",
            "账号开户时间":"open_time",
            "账户余额":"balance",
            "可用余额":"available_balance",
            "币种":"currency",
            "开户网点代码":"branch_code",
            "开户网点":"branch_name",
            "账户状态":"acct_status",
            "钞汇标志名称":"cashfx_flag_name",
            "销户日期":"close_date",
            "账户类型":"acct_type",
            "备注":"remark",
            "账号开户银行":"open_bank",
            "销户网点":"close_branch",
            "最后交易时间":"last_txn_time",
        },
    ),
    "fc_person": FcSchema(
        table="fc_person",
        headers=[
            "客户名称","证照类型","证照号码","单位地址","单位电话","工作单位","邮箱地址","代办人姓名",
            "代办人证件类型","代办人证件号码","国税纳税号","地税纳税号","法人代表","客户工商执照号码"
        ],
        col_map={
            "客户名称":"customer_name",
            "证照类型":"id_type",
            "证照号码":"id_no",
            "单位地址":"org_addr",
            "单位电话":"org_phone",
            "工作单位":"employer",
            "邮箱地址":"email",
            "代办人姓名":"agent_name",
            "代办人证件类型":"agent_id_type",
            "代办人证件号码":"agent_id_no",
            "国税纳税号":"nat_tax_no",
            "地税纳税号":"local_tax_no",
            "法人代表":"legal_rep",
            "客户工商执照号码":"license_no",
        },
    ),
    "fc_coercive_measure": FcSchema(
        table="fc_coercive_measure",
        headers=["银行名称","账号","冻结措施类型","冻结金额","冻结机关","冻结开始日期","冻结截止日期","措施序号","备注"],
        col_map={
            "银行名称":"bank_name",
            "账号":"acct_no",
            "冻结措施类型":"measure_type",
            "冻结金额":"amount",
            "冻结机关":"agency",
            "冻结开始日期":"start_date",
            "冻结截止日期":"end_date",
            "措施序号":"measure_seq_no",
            "备注":"remark",
        },
    ),
    "fc_transaction": FcSchema(
        table="fc_transaction",
        headers=[
            "交易卡号","交易账号","账户开户名称","开户人证件号码","交易时间","交易金额","交易余额","收付标志","交易对手账卡号","现金标志","对手户名",
            "对手身份证号","对手开户银行","摘要说明","交易币种","交易网点名称","交易网点代码","交易发生地","交易是否成功","传票号",
            "终端号","IP地址","MAC地址","对手交易余额","交易流水号","日志号","凭证种类","凭证号","交易柜员号","商户名称","商户号",
            "备注","交易类型","查询反馈结果原因"
        ],
        col_map={
            "交易卡号":"card_no",
            "交易账号":"acct_no",
            "账户开户名称":"account_open_name",
            "开户人证件号码":"opener_id_no",
            "交易时间":"txn_time",
            "交易金额":"amount",
            "交易余额":"balance",
            "收付标志":"dc_flag",
            "交易对手账卡号":"counterparty_acct",
            "现金标志":"cash_flag",
            "对手户名":"counterparty_name",
            "对手身份证号":"counterparty_id_no",
            "对手开户银行":"counterparty_bank",
            "摘要说明":"summary",
            "交易币种":"currency",
            "交易网点名称":"branch_name",
            "交易网点代码":"branch_code",
            "交易发生地":"location",
            "交易是否成功":"is_success",
            "传票号":"voucher_no",
            "终端号":"terminal_no",
            "IP地址":"ip_addr",
            "MAC地址":"mac_addr",
            "对手交易余额":"counterparty_balance",
            "交易流水号":"txn_id",
            "日志号":"log_id",
            "凭证种类":"voucher_type",
            "凭证号":"voucher_id",
            "交易柜员号":"teller_no",
            "商户名称":"merchant_name",
            "商户号":"merchant_no",
            "备注":"remark",
            "交易类型":"txn_type",
            "查询反馈结果原因":"query_feedback_reason",
        },
        store_raw_json=False
    ),
    "fc_sub_account": FcSchema(
        table="fc_sub_account",
        headers=["银行名称","开户账号","子账户账号","余额","可用余额","子账户类别","子账户序号","币种","钞汇标识","账户状态","账户序号"],
        col_map={
            "银行名称":"bank_name",
            "开户账号":"parent_acct",
            "子账户账号":"sub_acct",
            "余额":"balance",
            "可用余额":"available_balance",
            "子账户类别":"sub_type",
            "子账户序号":"sub_seq_no",
            "币种":"currency",
            "钞汇标识":"cashfx_flag",
            "账户状态":"acct_status",
            "账户序号":"acct_seq_no",
        },
    ),
    "fc_person_address": FcSchema(
        table="fc_person_address",
        headers=["开户名称","证照类型","证照号码","住宅地址","住宅电话"],
        col_map={
            "开户名称":"open_name",
            "证照类型":"id_type",
            "证照号码":"id_no",
            "住宅地址":"home_addr",
            "住宅电话":"home_phone",
        },
    ),
    "fc_person_contact": FcSchema(
        table="fc_person_contact",
        headers=["开户名称","证照类型","证照号码","联系电话"],
        col_map={
            "开户名称":"open_name",
            "证照类型":"id_type",
            "证照号码":"id_no",
            "联系电话":"contact_phone",
        },
    ),
    "fc_task_success": FcSchema(
        table="fc_task_success",
        headers=["任务流水号","银行名称","主体类别","证账号码","账卡号","发送时间","反馈时间","反馈结果","反馈非明细结果","反馈明细结果","入库时间","入库状态","请求单号","查询结果"],
        col_map={
            "任务流水号":"task_serial_no",
            "银行名称":"bank_name",
            "主体类别":"subject_type",
            "证账号码":"id_or_acct_no",
            "账卡号":"acct_card_no",
            "发送时间":"send_time",
            "反馈时间":"feedback_time",
            "反馈结果":"feedback_result",
            "反馈非明细结果":"feedback_non_detail",
            "反馈明细结果":"feedback_detail",
            "入库时间":"store_time",
            "入库状态":"store_status",
            "请求单号":"request_no",
            "查询结果":"query_result",
        },
    ),
    "fc_task_fail": FcSchema(
        table="fc_task_fail",
        headers=["任务流水号","银行名称","主体类别","证账号码","账卡号","发送时间","反馈时间","反馈结果","反馈非明细结果","反馈明细结果","入库时间","入库状态","请求单号","查询结果"],
        col_map={
            "任务流水号":"task_serial_no",
            "银行名称":"bank_name",
            "主体类别":"subject_type",
            "证账号码":"id_or_acct_no",
            "账卡号":"acct_card_no",
            "发送时间":"send_time",
            "反馈时间":"feedback_time",
            "反馈结果":"feedback_result",
            "反馈非明细结果":"feedback_non_detail",
            "反馈明细结果":"feedback_detail",
            "入库时间":"store_time",
            "入库状态":"store_status",
            "请求单号":"request_no",
            "查询结果":"query_result",
        },
    ),
}
