import { createRouter, createWebHistory } from "vue-router";
import { authRequired, isAuthed } from "@/lib/auth";

const routes = [
  { path: "/login", name: "login", component: () => import("@/views/LoginView.vue") },

  { path: "/", name: "home", component: () => import("@/views/HomeView.vue") },

  { path: "/trainees", name: "trainees", component: () => import("@/views/TraineesView.vue") },
  { path: "/trainees/new", name: "trainee-new", component: () => import("@/views/TraineeFormView.vue") },
  { path: "/trainees/:id", name: "trainee-detail", component: () => import("@/views/TraineeDetailView.vue") },
  { path: "/trainees/:id/edit", name: "trainee-edit", component: () => import("@/views/TraineeFormView.vue") },

  { path: "/schedule", name: "schedule", component: () => import("@/views/ScheduleView.vue") },
  { path: "/schedule/new", name: "session-new", component: () => import("@/views/SessionFormView.vue") },
  { path: "/schedule/:id", name: "session-detail", component: () => import("@/views/SessionDetailView.vue") },
  { path: "/schedule/:id/edit", name: "session-edit", component: () => import("@/views/SessionFormView.vue") },

  { path: "/payments", name: "payments", component: () => import("@/views/PaymentsView.vue") },
  { path: "/payments/new", name: "payment-new", component: () => import("@/views/PaymentFormView.vue") },
  { path: "/payments/:id", name: "payment-detail", component: () => import("@/views/PaymentDetailView.vue") },
  { path: "/payments/:id/receipt", name: "payment-receipt", component: () => import("@/views/ReceiptView.vue") },

  { path: "/reminders", name: "reminders", component: () => import("@/views/RemindersView.vue") },
  { path: "/reminders/new", name: "reminder-new", component: () => import("@/views/ReminderFormView.vue") },
  { path: "/reminders/:id/edit", name: "reminder-edit", component: () => import("@/views/ReminderFormView.vue") },

  { path: "/financials", name: "financials", component: () => import("@/views/FinancialsView.vue") },
  { path: "/financials/reconcile", name: "reconcile", component: () => import("@/views/ReconcileView.vue") },
  { path: "/inventory", name: "inventory", component: () => import("@/views/InventoryView.vue") },
  { path: "/sales/:id", name: "sale-detail", component: () => import("@/views/SaleDetailView.vue") },
  { path: "/search", name: "search", component: () => import("@/views/SearchView.vue") },

  { path: "/:pathMatch(.*)*", redirect: "/" },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
  // Back/forward restores where the list was scrolled; new pages start at the top.
  scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 },
});

// Login gate: only when the API actually requires a token (API_TOKEN set on
// the server). Local dev and demo mode skip this entirely.
router.beforeEach(async (to) => {
  if (to.name === "login") return true;
  if ((await authRequired()) && !(await isAuthed())) {
    return { name: "login", query: { to: to.fullPath } };
  }
  return true;
});

export default router;
