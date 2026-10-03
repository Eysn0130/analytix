import { JsonReporter } from 'vitest/node'

// JSON alone omits interrupted runs. The shared regression owners use
// this reporter; keep Vitest's assertion evidence and add its completion state.
export default class SharedVitestReporter extends JsonReporter {
  constructor() {
    super({})
  }

  async onTestRunEnd(modules, errors, reason) {
    this.analytixRun = {
      reason,
      files: modules.map((module) => ({ name: module.moduleId, state: module.state() }))
    }
    await super.onTestRunEnd(modules, errors, reason)
  }

  writeReport(json) {
    return super.writeReport(JSON.stringify({ ...JSON.parse(json), analytixRun: this.analytixRun }))
  }
}
