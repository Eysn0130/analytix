const bootConfig = window.__ANALYTIX_FLOW_RUNTIME_BOOT__ || {};

function asList(value) {
  return Array.isArray(value) ? value.filter(Boolean).map((item) => String(item)) : [];
}

function notify(event, payload = {}) {
  if (typeof bootConfig.onEvent === "function") {
    try {
      bootConfig.onEvent({ event, ...payload });
    } catch {}
  }
}

function loadRuntimeScript(url) {
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    let settled = false;
    const finish = (error) => {
      if (settled) return;
      settled = true;
      if (timer) {
        window.clearTimeout(timer);
      }
      if (error) {
        notify("asset:error", { scriptUrl: url, error: error.message || String(error) });
        reject(error);
        return;
      }
      notify("asset:loaded", { scriptUrl: url });
      resolve();
    };
    const timer = window.setTimeout(() => {
      finish(new Error(`runtime asset load timed out: ${url}`));
    }, 12000);
    script.src = url;
    script.async = false;
    script.onload = () => finish();
    script.onerror = () => finish(new Error(`runtime asset load failed: ${url}`));
    notify("asset:start", { scriptUrl: url });
    document.body.appendChild(script);
  });
}

async function bootRuntimeAssets() {
  const scriptUrls = asList(bootConfig.scriptUrls);
  notify("boot:start", { count: scriptUrls.length });
  for (const scriptUrl of scriptUrls) {
    await loadRuntimeScript(scriptUrl);
  }
  await new Promise((resolve) => window.setTimeout(resolve, 0));
  if (typeof window.__analytixOnLoadFinished === "function") {
    window.__analytixOnLoadFinished();
  }
  notify("boot:ready", { count: scriptUrls.length });
}

window.__ANALYTIX_FLOW_RUNTIME_BOOT_PROMISE__ = bootRuntimeAssets().catch((error) => {
  window.__ANALYTIX_FLOW_RUNTIME_BOOT_ERROR__ = error?.message || String(error);
  throw error;
});
