(async function init() {
    const loginPane = document.getElementById("loginPane");
    const workspace = document.getElementById("workspace");
    const settingsPane = document.getElementById("settingsPane");
    const logsPane = document.getElementById("logsPane");
    const pageSize = 20;
    let logOffset = 0;
    let logQuery = "";

    function showPage(name) {
        document.querySelectorAll(".admin-nav .tab").forEach((t) => {
            t.classList.toggle("active", t.dataset.page === name);
        });
        settingsPane.classList.toggle("hidden", name !== "settings");
        logsPane.classList.toggle("hidden", name !== "logs");
        if (name === "logs") loadLogs();
    }

    async function loadLogs() {
        const body = document.getElementById("logBody");
        const info = document.getElementById("logPageInfo");
        const res = await api("/api/v1/admin/logs?q=" + encodeURIComponent(logQuery) + "&offset=" + logOffset + "&limit=" + pageSize);
        if (!res.ok) {
            body.innerHTML = '<tr><td colspan="7" class="hint">' + (res.message || "读取失败") + "</td></tr>";
            info.textContent = "—";
            return;
        }
        const items = (res.data && res.data.items) || [];
        const total = (res.data && res.data.total) || 0;
        if (!items.length) {
            body.innerHTML = '<tr><td colspan="7" class="hint">暂无记录</td></tr>';
        } else {
            body.innerHTML = items.map((r) => {
                const ad = escapeHtml(r.ad_account || "—");
                const name = r.display_name ? '<div class="log-name">' + escapeHtml(r.display_name) + "</div>" : "";
                const os = escapeHtml(r.os || "—");
                const ua = escapeHtml(r.user_agent || "");
                return "<tr>" +
                    "<td>" + escapeHtml(r.time || "—") + "</td>" +
                    "<td>" + escapeHtml(r.ip || "—") + "</td>" +
                    "<td>" + escapeHtml(r.mac || "—") + "</td>" +
                    "<td title=\"" + ua + "\">" + os + "</td>" +
                    "<td><strong>" + ad + "</strong>" + name + "</td>" +
                    "<td>" + escapeHtml(r.operation || "—") + "</td>" +
                    "<td>" + escapeHtml(r.expire_at || "—") + "</td>" +
                    "</tr>";
            }).join("");
        }
        const page = Math.floor(logOffset / pageSize) + 1;
        const pages = Math.max(1, Math.ceil(total / pageSize));
        info.textContent = "第 " + page + " / " + pages + " 页，共 " + total + " 条";
        document.getElementById("logPrev").disabled = logOffset <= 0;
        document.getElementById("logNext").disabled = logOffset + pageSize >= total;
    }

    function escapeHtml(s) {
        return String(s).replace(/[&<>"'`]/g, (c) => ({
            "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;", "`": "&#96;",
        }[c]));
    }

    async function loadSettings() {
        const res = await api("/api/v1/admin/settings");
        if (!res.ok) {
            loginPane.hidden = false;
            workspace.hidden = true;
            const logoutBtn = document.getElementById("logoutBtn");
            if (logoutBtn) logoutBtn.hidden = true;
            return false;
        }
        loginPane.hidden = true;
        workspace.hidden = false;
        const logoutBtn = document.getElementById("logoutBtn");
        if (logoutBtn) logoutBtn.hidden = false;
        const d = res.data;
        document.getElementById("tlsNote").textContent = d.tls_note || "";
        document.getElementById("fsAppId").value = d.feishu.app_id || "";
        document.getElementById("fsRedirect").value = d.feishu.redirect_url || "";
        document.getElementById("fsUserBase").value = d.feishu.ad_user_base || "";
        document.getElementById("ldapHost").value = d.ldap.host || "";
        document.getElementById("ldapPort").value = d.ldap.port || 636;
        document.getElementById("ldapUser").value = d.ldap.username || "";
        document.getElementById("ldapBase").value = d.ldap.base || "";
        document.getElementById("ldapServerName").value = d.ldap.tls_server_name || "";
        document.getElementById("ldapCA").value = d.ldap.ca_file || "";
        document.getElementById("ldapTLS").checked = !!d.ldap.tls;
        document.getElementById("ldapSkip").checked = !!d.ldap.skip_verify;
        document.getElementById("ldapMaxDays").value = d.ldap.password_max_days || 90;
        document.getElementById("admUser").value = d.admin.username || "";
        return true;
    }

    document.getElementById("adminLoginForm").addEventListener("submit", async (e) => {
        e.preventDefault();
        document.getElementById("loginError").textContent = "";
        const res = await api("/api/v1/admin/login", {
            method: "POST",
            body: JSON.stringify({
                username: document.getElementById("adminUser").value.trim(),
                password: document.getElementById("adminPass").value,
            }),
        });
        if (!res.ok) {
            document.getElementById("loginError").textContent = res.message || "登录失败";
            return;
        }
        await loadSettings();
    });

    document.getElementById("logoutBtn").addEventListener("click", async () => {
        await api("/api/v1/admin/logout", { method: "POST", body: "{}" });
        loginPane.hidden = false;
        workspace.hidden = true;
        document.getElementById("logoutBtn").hidden = true;
        document.getElementById("adminPass").value = "";
        document.getElementById("loginError").textContent = "";
    });

    document.querySelectorAll(".admin-nav .tab").forEach((tab) => {
        tab.addEventListener("click", () => showPage(tab.dataset.page));
    });
    document.getElementById("logSearchBtn").addEventListener("click", () => {
        logQuery = document.getElementById("logQuery").value.trim();
        logOffset = 0;
        loadLogs();
    });
    document.getElementById("logQuery").addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
            e.preventDefault();
            document.getElementById("logSearchBtn").click();
        }
    });
    document.getElementById("logPrev").addEventListener("click", () => {
        logOffset = Math.max(0, logOffset - pageSize);
        loadLogs();
    });
    document.getElementById("logNext").addEventListener("click", () => {
        logOffset += pageSize;
        loadLogs();
    });

    document.getElementById("testFeishuBtn").addEventListener("click", async () => {
        document.getElementById("fsTestOk").textContent = "";
        document.getElementById("fsTestErr").textContent = "";
        const res = await api("/api/v1/admin/test/feishu", {
            method: "POST",
            body: JSON.stringify({
                app_id: document.getElementById("fsAppId").value.trim(),
                app_secret: document.getElementById("fsAppSecret").value,
            }),
        });
        if (res.ok) document.getElementById("fsTestOk").textContent = res.message;
        else document.getElementById("fsTestErr").textContent = res.message;
    });

    document.getElementById("testLdapBtn").addEventListener("click", async () => {
        document.getElementById("ldapTestOk").textContent = "";
        document.getElementById("ldapTestErr").textContent = "";
        const out = document.getElementById("ldapTestOut");
        const res = await api("/api/v1/admin/test/ldap", {
            method: "POST",
            body: JSON.stringify({
                host: document.getElementById("ldapHost").value.trim(),
                port: Number(document.getElementById("ldapPort").value),
                tls: document.getElementById("ldapTLS").checked,
                skip_verify: document.getElementById("ldapSkip").checked,
                username: document.getElementById("ldapUser").value.trim(),
                password: document.getElementById("ldapPass").value,
                base: document.getElementById("ldapBase").value.trim(),
                ca_file: document.getElementById("ldapCA").value.trim(),
                tls_server_name: document.getElementById("ldapServerName").value.trim(),
            }),
        });
        const data = res.data || {};
        out.hidden = false;
        out.textContent = JSON.stringify(data, null, 2);
        if (data.ok) document.getElementById("ldapTestOk").textContent = res.message;
        else document.getElementById("ldapTestErr").textContent = res.message || "测试失败";
    });

    document.getElementById("settingsForm").addEventListener("submit", async (e) => {
        e.preventDefault();
        document.getElementById("saveOk").textContent = "";
        document.getElementById("saveErr").textContent = "";
        const body = {
            feishu: {
                app_id: document.getElementById("fsAppId").value.trim(),
                app_secret: document.getElementById("fsAppSecret").value,
                redirect_url: document.getElementById("fsRedirect").value.trim(),
                ad_user_base: document.getElementById("fsUserBase").value.trim(),
            },
            ldap: {
                host: document.getElementById("ldapHost").value.trim(),
                port: Number(document.getElementById("ldapPort").value),
                tls: document.getElementById("ldapTLS").checked,
                skip_verify: document.getElementById("ldapSkip").checked,
                username: document.getElementById("ldapUser").value.trim(),
                password: document.getElementById("ldapPass").value,
                base: document.getElementById("ldapBase").value.trim(),
                ca_file: document.getElementById("ldapCA").value.trim(),
                tls_server_name: document.getElementById("ldapServerName").value.trim(),
                password_max_days: Number(document.getElementById("ldapMaxDays").value),
            },
            admin: {
                username: document.getElementById("admUser").value.trim(),
                password: document.getElementById("admPass").value,
            },
        };
        const res = await api("/api/v1/admin/settings", {
            method: "PUT",
            body: JSON.stringify(body),
        });
        if (res.ok) {
            document.getElementById("saveOk").textContent = res.message;
            document.getElementById("fsAppSecret").value = "";
            document.getElementById("ldapPass").value = "";
            document.getElementById("admPass").value = "";
        } else {
            document.getElementById("saveErr").textContent = res.message || "保存失败";
        }
    });

    await loadSettings();
})();
