import type { ReactElement } from 'react'
import analytixWordmark from '../../../../asset/brand/analytix-logo-transparent.png'
import analytixWordmarkReversed from '../../../../asset/brand/analytix-logo-transparent-reversed.png'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'
import { AnalytixBrandMark } from '../brand/AnalytixBrandMark'

/**
 * 迷你工作台舞台:标题栏 + 骨架画布 + 居中 analytix 品牌标识。
 *
 * `waking` 打开「唤醒中」加载特效:Zzz 睡息、声纳涟漪、气泡上浮、
 * 草稿打字光标和偶尔翻身。仅在运行时连接页(确实还在重连)启用;
 * 就绪后的空状态与报错态沿用安静版舞台,避免误读成仍在加载(#78)。
 */
export function AnalytixHeroStage({ waking = false }: { waking?: boolean }): ReactElement {
  return (
    <div
      className={waking ? 'ds-runtime-wake-stage is-waking' : 'ds-runtime-wake-stage'}
      aria-hidden="true"
    >
      <div className="ds-runtime-wake-shell">
        <div className="ds-runtime-wake-titlebar">
          <span className="ds-runtime-wake-dot is-red" />
          <span className="ds-runtime-wake-dot is-yellow" />
          <span className="ds-runtime-wake-dot is-green" />
          <span className="ds-runtime-wake-brand">
            <img
              className="ds-runtime-wake-brand-wordmark ds-runtime-wake-brand-wordmark-light"
              src={analytixWordmark}
              alt=""
              draggable={false}
              decoding="async"
            />
            <img
              className="ds-runtime-wake-brand-wordmark ds-runtime-wake-brand-wordmark-dark"
              src={analytixWordmarkReversed}
              alt=""
              draggable={false}
              decoding="async"
            />
          </span>
          <AnalytixIconRegistry.icons.panel className="ml-auto h-3.5 w-3.5 text-ds-faint" strokeWidth={1.7} />
        </div>
        <div className="ds-runtime-wake-body">
          <div className="ds-runtime-wake-nav">
            <span className="ds-runtime-wake-nav-line is-wide" />
            <span className="ds-runtime-wake-nav-line is-medium" />
            <div className="ds-runtime-wake-nav-tree is-one">
              <span className="ds-runtime-wake-nav-parent" />
              <span className="ds-runtime-wake-nav-child is-primary" />
              <span className="ds-runtime-wake-nav-child is-secondary" />
            </div>
            <div className="ds-runtime-wake-nav-tree is-two">
              <span className="ds-runtime-wake-nav-parent" />
              <span className="ds-runtime-wake-nav-child is-primary" />
              <span className="ds-runtime-wake-nav-child is-secondary" />
            </div>
            <div className="ds-runtime-wake-nav-matrix">
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
              <i />
            </div>
          </div>
          <div className="ds-runtime-wake-canvas">
            <span className="ds-runtime-wake-horizon" />
            <span className="ds-runtime-wake-terrain" />
            <span className="ds-runtime-wake-calibration is-left" />
            <span className="ds-runtime-wake-calibration is-right" />
            <span className="ds-runtime-wake-anchor is-one" />
            <span className="ds-runtime-wake-anchor is-two" />
            <span className="ds-runtime-wake-anchor is-three" />
            <span className="ds-runtime-wake-focus">
              <i className="is-top-left" />
              <i className="is-top-right" />
              <i className="is-bottom-left" />
              <i className="is-bottom-right" />
            </span>
            <span className="ds-runtime-wake-orbit" />
            <span className="ds-runtime-wake-thread is-one" />
            <span className="ds-runtime-wake-thread is-two" />
            <span className="ds-runtime-wake-thread is-three" />
            {waking ? (
              <span className="ds-runtime-wake-bubbles">
                <i />
                <i />
                <i />
                <i />
              </span>
            ) : null}
          </div>
          <div className="ds-runtime-wake-tools">
            <i />
            <i />
            <i />
            <i />
            <span />
            <i />
            <i />
          </div>
        </div>
        <span className="ds-runtime-wake-flow is-left" />
        <span className="ds-runtime-wake-flow is-right" />
        <div className="ds-runtime-wake-composer">
          <span />
          {waking ? <i className="ds-runtime-wake-caret" /> : null}
          <span />
        </div>
        <div className="ds-runtime-wake-core">
          {waking ? (
            <>
              <span className="ds-runtime-wake-sonar is-one" />
              <span className="ds-runtime-wake-sonar is-two" />
            </>
          ) : null}
          <span className="ds-runtime-wake-ring" />
          <span className="ds-runtime-wake-analytix-bob">
            <AnalytixBrandMark className="ds-runtime-wake-analytix" />
            {waking ? (
              <span className="ds-runtime-wake-zzz">
                <i>z</i>
                <i>z</i>
                <i>z</i>
              </span>
            ) : null}
          </span>
        </div>
      </div>
    </div>
  )
}
