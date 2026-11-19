async function api(url, options = {}) {
    const headers = Object.assign({ "Content-Type": "application/json" }, options.headers || {});
    const res = await fetch(url, Object.assign({ credentials: "same-origin" }, options, { headers }));
    let body = {};
    try {
        body = await res.json();
    } catch (e) {
        body = { code: -1, message: "服务器返回异常" };
    }
    return {
        ok: res.ok && body.code === 0,
        status: res.status,
        code: body.code,
        message: body.message || "",
        hint: body.hint || "",
        data: body.data,
    };
}

function queryParam(name) {
    return new URLSearchParams(window.location.search).get(name) || "";
}

function showModal(title, message, isError) {
    const mask = document.getElementById("appModal");
    if (!mask) {
        window.alert(message);
        return Promise.resolve();
    }
    document.getElementById("modalTitle").textContent = title || "提示";
    document.getElementById("modalMsg").textContent = message || "";
    mask.classList.toggle("is-error", !!isError);
    mask.classList.remove("hidden");
    return new Promise((resolve) => {
        const btn = document.getElementById("modalOk");
        const done = () => {
            mask.classList.add("hidden");
            btn.removeEventListener("click", done);
            document.removeEventListener("keydown", onKey);
            resolve();
        };
        const onKey = (e) => {
            if (e.key === "Escape") done();
        };
        btn.addEventListener("click", done);
        document.addEventListener("keydown", onKey);
        btn.focus();
    });
}
