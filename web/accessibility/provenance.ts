import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { arch, platform, release } from 'node:os'
import { join } from 'node:path'
import type { FullConfig, FullResult, Reporter, TestCase, TestResult } from '@playwright/test/reporter'

function source() {
  return { commit: execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim(), clean: execFileSync('git', ['status', '--porcelain', '--untracked-files=normal'], { encoding: 'utf8' }).trim() === '' }
}
export default class Provenance implements Reporter {
  private start = source()
  private output = ''
  private results: { title: string; status: string; duration_ms: number }[] = []
  onBegin(config: FullConfig) { this.output = config.projects[0].outputDir }
  onTestEnd(test: TestCase, result: TestResult) { this.results.push({ title: test.titlePath().join(' > '), status: result.status, duration_ms: result.duration }) }
  onEnd(result: FullResult) {
    const end = source()
    mkdirSync(this.output, { recursive: true })
    writeFileSync(join(this.output, 'source-environment.json'), JSON.stringify({
      source_start: this.start, source_end: end, same_commit: this.start.commit === end.commit,
      overall: result.status, duration_ms: result.duration,
      host: { os: platform(), release: release(), architecture: arch(), node: process.version },
      evidence_kind: 'supplementary automated engine and keyboard checks; no manual or retail-device pass implied',
      tests: this.results,
    }, null, 2), { mode: 0o600 })
  }
}
