import { useEffect, useRef, useState } from "react";
import { AnimatedCharacters } from "./components";

const QR_SDK =
  "https://lf-package-cn.feishucdn.com/obj/feishu-static/lark/passport/qrcode/LarkSSOSDKWebQRCode-1.0.3.js";

const ERROR_TEXT = {
  invalid_state: "登录校验失败，请重新扫码。",
  missing_code: "飞书未返回授权码，请重试。",
  feishu_token_failed: "飞书授权失败，请稍后重试。",
  feishu_userinfo_failed: "获取飞书用户信息失败。",
  feishu_name_empty: "飞书姓名为空，无法匹配 AD 账号，请联系管理员。",
  ad_connect_failed: "连接 AD 失败，请联系管理员。",
  ad_user_not_found: "未在 AD 中找到对应用户，请确认飞书姓名与 AD 显示名一致。",
  ad_user_ambiguous: "AD 中存在多个同名用户，无法唯一匹配，请联系管理员。",
  session_save_failed: "写入登录态失败，请重试。",
  generate_state_failed: "初始化登录失败，请刷新页面重试。",
  unauthorized: "登录已过期，请重新登录。",
  feishu_denied: "已取消飞书授权。",
  ad_login_failed: "AD 登录失败。",
};

function loadScript(src) {
  return new Promise((resolve, reject) => {
    const existing = document.querySelector(`script[src="${src}"]`);
    if (existing) {
      resolve();
      return;
    }
    const script = document.createElement("script");
    script.src = src;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("script"));
    document.head.appendChild(script);
  });
}

async function api(url, options = {}) {
  const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
  const res = await fetch(url, { credentials: "same-origin", ...options, headers });
  let body = {};
  try {
    body = await res.json();
  } catch {
    body = { code: -1, message: "服务器返回异常" };
  }
  return {
    ok: res.ok && body.code === 0,
    message: body.message || "",
    data: body.data,
  };
}

function App() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const [isPasswordFocused, setIsPasswordFocused] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errorMsg, setErrorMsg] = useState("");
  const [usernameError, setUsernameError] = useState(false);
  const [passwordError, setPasswordError] = useState(false);
  const [loginFailed, setLoginFailed] = useState(false);
  const [loginSuccess, setLoginSuccess] = useState(false);
  const [showQr, setShowQr] = useState(false);
  const [qrHint, setQrHint] = useState("请使用飞书扫描二维码");
  const [session, setSession] = useState(null);
  const qrStarted = useRef(false);

  const fail = (message) => {
    setErrorMsg(message);
    setLoginFailed(true);
    window.setTimeout(() => setLoginFailed(false), 2000);
  };

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const hint = params.get("error") || "";
    if (hint) {
      const name = params.get("name") || "";
      const msg = params.get("msg") || "";
      let text = ERROR_TEXT[hint] || msg || "登录失败，请重试。";
      if (hint === "ad_user_not_found" && name) {
        text += `（飞书姓名：${name}）`;
      } else if (msg && ERROR_TEXT[hint]) {
        text += ` ${msg}`;
      }
      setErrorMsg(text);
      setLoginFailed(true);
      window.setTimeout(() => setLoginFailed(false), 2000);
    }

    api("/api/v1/auth/me").then((me) => {
      if (me.ok && me.data) setSession(me.data);
    });
  }, []);

  useEffect(() => {
    if (!showQr || qrStarted.current) return undefined;
    let cancelled = false;

    (async () => {
      const cfg = await api("/api/v1/auth/feishu/qr-config");
      if (cancelled) return;
      if (!cfg.ok || !cfg.data?.goto) {
        setQrHint("二维码初始化失败，请改用「飞书一键授权」。");
        return;
      }
      try {
        await loadScript(QR_SDK);
      } catch {
        if (!cancelled) setQrHint("二维码组件加载失败，请改用「飞书一键授权」。");
        return;
      }
      if (cancelled || typeof window.QRLogin !== "function") {
        if (!cancelled) setQrHint("二维码组件不可用，请改用「飞书一键授权」。");
        return;
      }
      qrStarted.current = true;
      const gotoURL = cfg.data.goto;
      const qr = window.QRLogin({
        id: "login_container",
        goto: gotoURL,
        width: "268",
        height: "268",
        style: "width:268px;height:268px;border:none;background:#fff;",
      });
      const onMessage = (event) => {
        if (qr.matchOrigin(event.origin) && qr.matchData(event.data)) {
          window.location.href = `${gotoURL}&tmp_code=${event.data.tmp_code}`;
        }
      };
      window.addEventListener("message", onMessage);
    })();

    return () => {
      cancelled = true;
    };
  }, [showQr]);

  const onSubmit = async (event) => {
    event.preventDefault();
    setUsernameError(false);
    setPasswordError(false);
    setErrorMsg("");
    setLoginFailed(false);
    setLoginSuccess(false);

    const name = username.trim();
    if (!name) {
      setUsernameError(true);
      fail("请输入 AD 登录名。");
      return;
    }
    if (!password) {
      setPasswordError(true);
      fail("请输入密码。");
      return;
    }

    setIsSubmitting(true);
    try {
      const result = await api("/api/v1/auth/ad/login", {
        method: "POST",
        body: JSON.stringify({ username: name, password }),
      });
      if (result.ok) {
        setLoginSuccess(true);
        window.location.href = "/console.html";
        return;
      }
      setPasswordError(true);
      fail(result.message || "登录失败");
    } catch {
      fail("网络错误，请稍后重试。");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div id="login-page">
      <div className="left-panel">
        <div className="logo">
          <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2">
            <path d="M12 2L15 9H9L12 2Z" />
            <path d="M12 22L9 15H15L12 22Z" />
            <path d="M2 12L9 9V15L2 12Z" />
            <path d="M22 12L15 15V9L22 12Z" />
          </svg>
          <span>密码自助</span>
        </div>

        <div className="characters-wrapper">
          <AnimatedCharacters
            isTyping={isTyping}
            isPasswordFocused={isPasswordFocused}
            showPassword={showPassword}
            passwordLength={password.length}
            loginFailed={loginFailed}
            loginSuccess={loginSuccess}
          />
        </div>

        <div className="footer-links">
          <span>Active Directory 密码自助</span>
        </div>
      </div>

      <div className="right-panel">
        <div className="form-container">
          <div className="sparkle-icon">
            <svg viewBox="0 0 24 24" fill="none">
              <path d="M12 2L13.5 9H10.5L12 2Z" fill="#1a1a2e" />
              <path d="M12 22L10.5 15H13.5L12 22Z" fill="#1a1a2e" />
              <path d="M2 12L9 10.5V13.5L2 12Z" fill="#1a1a2e" />
              <path d="M22 12L15 13.5V10.5L22 12Z" fill="#1a1a2e" />
            </svg>
          </div>

          <div className="form-header">
            <h1>欢迎回来</h1>
            <p>使用 AD 账号登录，或通过飞书验证身份</p>
          </div>

          {session ? (
            <p className="session-note">
              已登录：<strong>{session.display_name || "用户"}</strong>
              ，登录名 <strong>{session.login_name || session.sam || ""}</strong>。{" "}
              <a href="/console.html">进入自助中心</a>
            </p>
          ) : null}

          <form onSubmit={onSubmit}>
            <div className="form-group">
              <label htmlFor="username" className={usernameError ? "error-label" : ""}>
                AD 登录名
              </label>
              <div className="input-wrapper">
                <input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(event) => {
                    setUsername(event.target.value);
                    if (usernameError) setUsernameError(false);
                    if (errorMsg) setErrorMsg("");
                  }}
                  onFocus={() => setIsTyping(true)}
                  onBlur={() => setIsTyping(false)}
                  placeholder="zhangsan"
                  autoComplete="username"
                  className={usernameError ? "error" : ""}
                />
              </div>
            </div>

            <div className="form-group">
              <label htmlFor="password" className={passwordError ? "error-label" : ""}>
                密码
              </label>
              <div className="input-wrapper">
                <input
                  id="password"
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(event) => {
                    setPassword(event.target.value);
                    if (passwordError) setPasswordError(false);
                    if (errorMsg) setErrorMsg("");
                  }}
                  onFocus={() => setIsPasswordFocused(true)}
                  onBlur={() => setIsPasswordFocused(false)}
                  placeholder="••••••••"
                  autoComplete="current-password"
                  className={passwordError ? "error" : ""}
                />
                <button
                  type="button"
                  className="toggle-password"
                  onClick={() => setShowPassword((prev) => !prev)}
                  aria-label={showPassword ? "隐藏密码" : "显示密码"}
                >
                  {showPassword ? (
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                      <line x1="1" y1="1" x2="23" y2="23" />
                    </svg>
                  ) : (
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                      <circle cx="12" cy="12" r="3" />
                    </svg>
                  )}
                </button>
              </div>
            </div>

            {errorMsg ? <div className="error-msg show">{errorMsg}</div> : null}

            {showQr ? (
              <div className="qr-panel">
                <div id="login_container" />
                <p className="form-hint">{qrHint}</p>
              </div>
            ) : null}

            <button type="submit" className="btn-login" disabled={isSubmitting}>
              <span className="btn-text">{isSubmitting ? "正在登录…" : "登录"}</span>
              <div className="btn-hover-content">
                <span>{isSubmitting ? "正在登录…" : "登录"}</span>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                  <line x1="5" y1="12" x2="19" y2="12" />
                  <polyline points="12 5 19 12 12 19" />
                </svg>
              </div>
            </button>

            <button
              type="button"
              className="btn-secondary"
              onClick={() => {
                window.location.href = "/api/v1/auth/feishu/login";
              }}
            >
              <span className="btn-text">飞书一键授权</span>
              <div className="btn-hover-content">
                <span>飞书一键授权</span>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                  <line x1="5" y1="12" x2="19" y2="12" />
                  <polyline points="12 5 19 12 12 19" />
                </svg>
              </div>
            </button>

            <button type="button" className="text-button" onClick={() => setShowQr((prev) => !prev)}>
              {showQr ? "收起飞书二维码" : "忘记密码或账户锁定？飞书扫码"}
            </button>
          </form>

          <p className="signup-link">
            <a href="/admin.html">管理后台</a>
          </p>
        </div>
      </div>
    </div>
  );
}

export default App;
