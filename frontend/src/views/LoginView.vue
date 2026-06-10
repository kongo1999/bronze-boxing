<script setup lang="ts">
import { ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Lock } from "lucide-vue-next";
import { setToken, verifyToken } from "@/lib/auth";
import { clearCache } from "@/lib/cache";
import Button from "@/components/ui/Button.vue";
import { inputCls } from "@/lib/ui";

const route = useRoute();
const router = useRouter();

const token = ref("");
const checking = ref(false);
const error = ref("");

async function submit() {
  const t = token.value.trim();
  if (!t || checking.value) return;
  checking.value = true;
  error.value = "";
  if (await verifyToken(t)) {
    setToken(t);
    clearCache();
    const to = typeof route.query.to === "string" ? route.query.to : "/";
    router.push(to);
  } else {
    error.value = "That token didn't work. Check it and try again.";
    checking.value = false;
  }
}
</script>

<template>
  <div class="grid min-h-dvh place-items-center px-4">
    <div class="w-full max-w-sm space-y-6">
      <div class="space-y-2 text-center">
        <div class="mx-auto grid h-12 w-12 place-items-center rounded-2xl bg-purple/15 text-purple">
          <Lock class="h-6 w-6" />
        </div>
        <h1 class="font-display text-2xl font-semibold">Bronze Boxing</h1>
        <p class="text-sm text-muted">Enter the studio access token to continue.</p>
      </div>

      <form class="space-y-3" @submit.prevent="submit">
        <input
          v-model="token"
          type="password"
          autocomplete="current-password"
          placeholder="Access token"
          :class="inputCls"
          autofocus
        />
        <p v-if="error" class="text-sm text-overdue">{{ error }}</p>
        <Button class="w-full" :disabled="checking || !token.trim()" type="submit">
          {{ checking ? "Checking…" : "Unlock" }}
        </Button>
      </form>

      <p class="text-center text-xs text-faint">
        The token is set by the studio admin (API_TOKEN on the server).
      </p>
    </div>
  </div>
</template>
