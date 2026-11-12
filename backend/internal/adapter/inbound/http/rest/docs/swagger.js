// Apply the saved theme before the page is painted. Storage may be blocked by
// the browser; switching themes still works for the current visit in that case.
(() => {
  const storageKey = "dataset.swagger.theme";
  const root = document.documentElement;
  let theme = matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  try {
    const saved = localStorage.getItem(storageKey);
    if (saved === "light" || saved === "dark") theme = saved;
  } catch (_) { /* Keep the system theme when storage is unavailable. */ }
  root.dataset.swaggerTheme = theme;

  const mount = () => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "swagger-theme-toggle";
    button.setAttribute("aria-label", "Dark mode");
    button.title = theme === "dark" ? "Switch to light mode" : "Switch to dark mode";
    button.setAttribute("aria-pressed", String(theme === "dark"));
    button.addEventListener("click", () => {
      theme = theme === "dark" ? "light" : "dark";
      root.dataset.swaggerTheme = theme;
      button.setAttribute("aria-pressed", String(theme === "dark"));
      button.title = theme === "dark" ? "Switch to light mode" : "Switch to dark mode";
      try {
        localStorage.setItem(storageKey, theme);
      } catch (_) { /* The theme has already changed; persistence is optional. */ }
    });
    // Swagger renders its title asynchronously and may replace it on spec reload.
    const swagger = document.getElementById("swagger-ui");
    if (!swagger) return;
    const placeButton = () => {
      const title = swagger.querySelector(".info .title");
      if (title && button.parentElement !== title) title.append(button);
    };
    new MutationObserver(placeButton).observe(swagger, { childList: true, subtree: true });
    placeButton();
  };
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", mount, { once: true });
  } else {
    mount();
  }
})();

// Swagger UI has no tag groups: read x-tagGroups from the spec and put a title
// that opens and closes the group above its first tag.
function TagGroups() {
  return {
    wrapComponents: {
      OperationTag: (Original, system) => (props) => {
        const { createElement: h, Fragment } = system.React;
        const groups = system.specSelectors.specJson().get("x-tagGroups");
        const group = groups && groups.find((g) => g.get("tags").includes(props.tag));
        if (!group) return h(Original, props);

        const name = group.get("name");
        const key = ["tag-groups", name];
        const open = system.layoutSelectors.isShown(key, false);
        const toggle = () => system.layoutActions.show(key, !open);
        const count = group.get("tags").size;
        const title = group.getIn(["tags", 0]) === props.tag && h(
          "h2",
          { className: "tag-group" },
          h(
            "button",
            { type: "button", "aria-expanded": open, onClick: toggle },
            name,
            h("span", null, count === 1 ? "1 tag" : `${count} tags`),
          ),
        );

        // A filter shows its matches even in a closed group: only the first tag has the title.
        const filter = system.layoutSelectors.currentFilter(); // true until something is typed
        const visible = open || (typeof filter === "string" && filter !== "");
        return h(Fragment, null, title, visible && h("div", { className: "tag-group-body" }, h(Original, props)));
      },
    },
  };
}

// Log in or refresh in "Try it out" and the access token goes into Authorize,
// so protected routes work without copying it. Logging out clears it.
function saveToken(res) {
  const path = new URL(res.url, location.href).pathname;
  if (res.ok && (path === "/v1/auth/login" || path === "/v1/auth/refresh") && res.body && res.body.access_token) {
    window.ui.preauthorizeApiKey("BearerAuth", "Bearer " + res.body.access_token);
    window.ui.authActions.persistAuthorizationIfNeeded(); // preauthorize skips persistAuthorization
  } else if (res.ok && path === "/v1/auth/logout") {
    window.ui.authActions.logoutWithPersistOption(["BearerAuth"]);
  }
  return res;
}
