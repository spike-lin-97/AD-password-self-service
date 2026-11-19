function hasUpper(s) { return /[A-Z]/.test(s); }
function hasLower(s) { return /[a-z]/.test(s); }
function hasDigit(s) { return /\d/.test(s); }
function hasSpecial(s) { return /[^A-Za-z0-9]/.test(s); }

function complexityOk(password) {
    let n = 0;
    if (hasUpper(password)) n++;
    if (hasLower(password)) n++;
    if (hasDigit(password)) n++;
    if (hasSpecial(password)) n++;
    return n >= 3;
}

(async function init() {
    const welcome = document.getElementById("welcome");
    const form = document.getElementById("changePasswordForm");
    const unlockPane = document.getElementById("unlockPane");
    const newPassword = document.getElementById("newPassword");
    const confirmPassword = document.getElementById("confirmPassword");
    const passwordError = document.getElementById("passwordError");
    const confirmError = document.getElementById("confirmError");
    const successMessage = document.getElementById("successMessage");
    const formError = document.getElementById("formError");
    const submitBtn = document.getElementById("submitBtn");
    const copyBtn = document.getElementById("copyBtn");
    const unlockBtn = document.getElementById("unlockBtn");

    const me = await api("/api/v1/auth/me");
    if (!me.ok) {
        window.location.href = "/?error=unauthorized";
        return;
    }

    const loginName = me.data.login_name || me.data.sam || "";
    const displayName = me.data.display_name || "用户";
    welcome.textContent = "你好，" + displayName;
    document.getElementById("displayName").textContent = displayName;
    document.getElementById("loginName").textContent = loginName || "（未获取到登录名）";
    const badge = me.data.locked
        ? '<span class="badge badge-lock">已锁定</span>'
        : '<span class="badge badge-ok">可登录</span>';
    document.getElementById("displayName").innerHTML = displayName + " " + badge;
    document.getElementById("pwdLastSet").textContent = me.data.pwd_last_set || "未知";
    document.getElementById("pwdExpireAt").textContent = me.data.pwd_expire_at || "未知";
    document.getElementById("accountCard").hidden = false;

    copyBtn.addEventListener("click", async () => {
        try {
            await navigator.clipboard.writeText(loginName);
            copyBtn.textContent = "已复制";
            setTimeout(() => { copyBtn.textContent = "复制登录名"; }, 1600);
        } catch (e) {
            copyBtn.textContent = "复制失败";
        }
    });

    document.querySelectorAll(".tab").forEach((tab) => {
        tab.addEventListener("click", () => {
            document.querySelectorAll(".tab").forEach((t) => t.classList.remove("active"));
            tab.classList.add("active");
            const isUnlock = tab.dataset.tab === "unlock";
            form.classList.toggle("hidden", isUnlock);
            unlockPane.classList.toggle("hidden", !isUnlock);
        });
    });

    form.addEventListener("submit", async (e) => {
        e.preventDefault();
        passwordError.textContent = "";
        confirmError.textContent = "";
        successMessage.textContent = "";
        formError.textContent = "";

        const pwd = newPassword.value;
        const confirm = confirmPassword.value;
        let valid = true;
        if (pwd.length < 8) {
            passwordError.textContent = "密码长度不能少于 8 位";
            valid = false;
        } else if (!complexityOk(pwd)) {
            passwordError.textContent = "须包含大写、小写、数字、特殊字符中的至少 3 类";
            valid = false;
        }
        if (confirm !== pwd) {
            confirmError.textContent = "两次输入的密码不一致";
            valid = false;
        }
        if (!valid) return;

        submitBtn.disabled = true;
        try {
            const result = await api("/api/v1/auth/change-password", {
                method: "POST",
                body: JSON.stringify({ new_password: pwd }),
            });
            if (result.ok) {
                form.reset();
                const fresh = await api("/api/v1/auth/me");
                if (fresh.ok && fresh.data) {
                    document.getElementById("pwdLastSet").textContent = fresh.data.pwd_last_set || "未知";
                    document.getElementById("pwdExpireAt").textContent = fresh.data.pwd_expire_at || "未知";
                }
                await showModal("修改成功", result.message || "密码已修改，请使用新密码登录电脑和邮箱。");
            } else {
                await showModal("修改失败", result.message || "修改失败，请稍后重试", true);
            }
        } catch (err) {
            await showModal("修改失败", "网络错误，请稍后重试", true);
        } finally {
            submitBtn.disabled = false;
        }
    });

    unlockBtn.addEventListener("click", async () => {
        document.getElementById("unlockSuccess").textContent = "";
        document.getElementById("unlockError").textContent = "";
        unlockBtn.disabled = true;
        try {
            const result = await api("/api/v1/auth/unlock-account", { method: "POST", body: "{}" });
            if (result.ok) {
                document.getElementById("displayName").innerHTML = displayName + ' <span class="badge badge-ok">可登录</span>';
                await showModal("解锁成功", "账户 " + (result.data && result.data.login_name ? result.data.login_name : loginName) + " 已解锁，可以重新登录。");
            } else {
                await showModal("解锁失败", result.message || "解锁失败", true);
            }
        } catch (err) {
            await showModal("解锁失败", "网络错误，请稍后重试", true);
        } finally {
            unlockBtn.disabled = false;
        }
    });
})();
