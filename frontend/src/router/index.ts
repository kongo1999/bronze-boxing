import { createRouter, createWebHistory } from "vue-router";

const routes = [
  { path: "/", name: "dashboard", component: () => import("@/views/DashboardView.vue") },

  { path: "/trainees", name: "trainees", component: () => import("@/views/TraineesView.vue") },
  { path: "/trainees/new", name: "trainee-new", component: () => import("@/views/TraineeFormView.vue") },
  { path: "/trainees/:id", name: "trainee-detail", component: () => import("@/views/TraineeDetailView.vue") },
  { path: "/trainees/:id/edit", name: "trainee-edit", component: () => import("@/views/TraineeFormView.vue") },

  { path: "/schedule", name: "schedule", component: () => import("@/views/ScheduleView.vue") },
  { path: "/schedule/new", name: "session-new", component: () => import("@/views/SessionFormView.vue") },
  { path: "/schedule/:id", name: "session-detail", component: () => import("@/views/SessionDetailView.vue") },

  { path: "/payments", name: "payments", component: () => import("@/views/PaymentsView.vue") },
  { path: "/payments/new", name: "payment-new", component: () => import("@/views/PaymentFormView.vue") },

  { path: "/reminders", name: "reminders", component: () => import("@/views/RemindersView.vue") },
  { path: "/reminders/new", name: "reminder-new", component: () => import("@/views/ReminderFormView.vue") },

  { path: "/financials", name: "financials", component: () => import("@/views/FinancialsView.vue") },
  { path: "/inventory", name: "inventory", component: () => import("@/views/InventoryView.vue") },
  { path: "/search", name: "search", component: () => import("@/views/SearchView.vue") },

  { path: "/:pathMatch(.*)*", redirect: "/" },
];

export default createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
});
