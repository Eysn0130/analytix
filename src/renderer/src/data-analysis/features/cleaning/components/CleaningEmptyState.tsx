export function CleaningEmptyState(): JSX.Element {
  return (
    <div className="cleaning-dashboard cleaning-dashboard--empty">
      <div className="cleaning-empty-stage">
        <div className="cleaning-empty-state">
          <h2>数据清洗工作台</h2>
          <p>请先到案件页切换当前案件，再进入清洗页面查看链路、执行任务和检查步骤洞察。</p>
        </div>
      </div>
    </div>
  );
}
