<script setup lang="ts">
import { ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { login } from "@/lib/auth";
import logoUrl from "@/assets/logo.png";
import { clearCache } from "@/lib/cache";
import Button from "@/components/ui/Button.vue";
import { inputCls } from "@/lib/ui";

const route = useRoute();
const router = useRouter();

const username = ref("");
const password = ref("");
const remember = ref(true);
const checking = ref(false);
const error = ref("");

async function submit() {
  if (checking.value || !username.value.trim() || !password.value) return;
  checking.value = true;
  error.value = "";
  const err = await login(username.value.trim(), password.value, remember.value);
  if (err === null) {
    clearCache();
    const to = typeof route.query.to === "string" ? route.query.to : "/";
    router.push(to);
  } else {
    error.value = err;
    checking.value = false;
  }
}
</script>

<template>
  <div class="grid min-h-dvh place-items-center px-4">
    <div class="w-full max-w-sm space-y-6">
      <div class="space-y-3 text-center">
        <img :src="logoUrl" alt="Bronze Boxing Club" class="mx-auto h-28 w-auto" />
        <h1 class="font-display text-2xl font-semibold">Bronze Boxing</h1>
        <p class="text-sm text-muted">Sign in to manage the studio.</p>
      </div>

      <form class="space-y-3" @submit.prevent="submit">
        <input
          v-model="username"
          type="text"
          autocomplete="username"
          placeholder="Username"
          :class="inputCls"
          autofocus
        />
        <input
          v-model="password"
          type="password"
          autocomplete="current-password"
          placeholder="Password"
          :class="inputCls"
        />
        <label class="flex items-center gap-2 text-sm text-muted select-none">
          <input v-model="remember" type="checkbox" class="h-4 w-4 accent-[oklch(0.72_0.13_64)]" />
          Remember me on this device
        </label>
        <p v-if="error" class="text-sm text-overdue">{{ error }}</p>
        <Button class="w-full" :disabled="checking || !username.trim() || !password" type="submit">
          {{ checking ? "Signing in…" : "Sign in" }}
        </Button>
      </form>
    </div>
  </div>
</template>
