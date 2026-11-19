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

const QR_SDK = "https://lf-package-cn.feishucdn.com/obj/feishu-static/lark/passport/qrcode/LarkSSOSDKWebQRCode-1.0.3.js";

function loadScript(src) {
    return new Promise((resolve, reject) => {
        const s = document.createElement("script");
        s.src = src;
        s.onload = resolve;
        s.onerror = reject;
        document.head.appendChild(s);
    });
}

async function initQr() {
    const hint = document.getElementById("qrHint");
    const cfg = await api("/api/v1/auth/feishu/qr-config");
    if (!cfg.ok || !cfg.data || !cfg.data.goto) {
        hint.textContent = "二维码初始化失败，请改用「一键授权」。";
        return;
    }
    const gotoURL = cfg.data.goto;
    try {
        await loadScript(QR_SDK);
    } catch (e) {
        hint.textContent = "二维码组件加载失败，请改用「一键授权」。";
        return;
    }
    if (typeof QRLogin !== "function") {
        hint.textContent = "二维码组件不可用，请改用「一键授权」。";
        return;
    }
    const QRLoginObj = QRLogin({
        id: "login_container",
        goto: gotoURL,
        width: "268",
        height: "268",
        style: "width:268px;height:268px;border:none;",
    });
    window.addEventListener("message", function (event) {
        if (QRLoginObj.matchOrigin(event.origin) && QRLoginObj.matchData(event.data)) {
            window.location.href = gotoURL + "&tmp_code=" + event.data.tmp_code;
        }
    });
}

(async function init() {
    const banner = document.getElementById("errorBanner");
    const sessionBanner = document.getElementById("sessionBanner");
    const hint = queryParam("error");
    if (hint) {
        const name = queryParam("name");
        const msg = queryParam("msg");
        let text = ERROR_TEXT[hint] || msg || "登录失败，请重试。";
        if (hint === "ad_user_not_found" && name) {
            text += "（飞书姓名：" + name + "）";
        } else if (msg && ERROR_TEXT[hint]) {
            text += " " + msg;
        }
        banner.textContent = text;
        banner.hidden = false;
    }

    const me = await api("/api/v1/auth/me");
    if (me.ok && me.data) {
        const login = me.data.login_name || me.data.sam || "";
        sessionBanner.innerHTML =
            "已登录：<strong>" +
            (me.data.display_name || "用户") +
            "</strong>，登录名 <strong>" +
            login +
            "</strong>。<a href=\"/console.html\">进入自助中心</a>";
        sessionBanner.hidden = false;
    }

    function showMethod(method) {
        document.querySelectorAll(".method-tabs .tab").forEach((t) => {
            t.classList.toggle("active", t.dataset.method === method);
        });
        document.getElementById("stage-qr").classList.toggle("hidden", method !== "qr");
        document.getElementById("stage-ad").classList.toggle("hidden", method !== "ad");
        if (method === "ad") {
            const user = document.getElementById("username");
            if (user) user.focus();
        }
    }

    document.querySelectorAll(".method-tabs .tab").forEach((tab) => {
        tab.addEventListener("click", (e) => {
            const method = tab.dataset.method;
            if (method === "client") {
                return;
            }
            e.preventDefault();
            showMethod(method);
        });
    });

    initQr();

    const form = document.getElementById("adLoginForm");
    const adError = document.getElementById("adError");
    const adLoginBtn = document.getElementById("adLoginBtn");
    form.addEventListener("submit", async (e) => {
        e.preventDefault();
        adError.textContent = "";
        adLoginBtn.disabled = true;
        try {
            const result = await api("/api/v1/auth/ad/login", {
                method: "POST",
                body: JSON.stringify({
                    username: document.getElementById("username").value.trim(),
                    password: document.getElementById("password").value,
                }),
            });
            if (result.ok) {
                window.location.href = "/console.html";
                return;
            }
            adError.textContent = result.message || "登录失败";
        } catch (err) {
            adError.textContent = "网络错误，请稍后重试";
        } finally {
            adLoginBtn.disabled = false;
        }
    });
})();
