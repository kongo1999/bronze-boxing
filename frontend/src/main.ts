import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import router from "./router";
// Self-hosted fonts (no external Google Fonts request; works offline).
import "@fontsource/geist-sans/400.css";
import "@fontsource/geist-sans/500.css";
import "@fontsource/geist-sans/600.css";
import "@fontsource/geist-sans/700.css";
import "@fontsource/oswald/500.css";
import "@fontsource/oswald/600.css";
import "@fontsource/oswald/700.css";
import "./style.css";

// Mount only after the initial navigation (and its async auth guard) settles,
// so the first paint is the resolved route — no flash of the app shell before
// an unauthenticated visitor is redirected to /login.
const app = createApp(App).use(createPinia()).use(router);
router.isReady().then(() => app.mount("#app"));
