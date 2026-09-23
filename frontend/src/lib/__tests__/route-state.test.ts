import { describe, expect, it } from "vitest";
import { defineComponent, h, nextTick } from "vue";
import { mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { backTarget, useQueryState, withQuery } from "../route-state";

async function setup(initial = "/list") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/list", component: { render: () => null } }],
  });
  await router.push(initial);
  await router.isReady();
  let api!: { kind: ReturnType<typeof useQueryState>; type: ReturnType<typeof useQueryState> };
  const Probe = defineComponent({
    setup() {
      api = {
        kind: useQueryState("kind", () => "all"),
        type: useQueryState("type", () => ""),
      };
      return () => h("div");
    },
  });
  mount(Probe, { global: { plugins: [router] } });
  return { router, api };
}

const settle = async () => {
  for (let i = 0; i < 5; i++) {
    await nextTick();
    await new Promise((r) => setTimeout(r, 0));
  }
};

describe("useQueryState", () => {
  it("reads its value from the URL", async () => {
    const { api } = await setup("/list?kind=expense&type=rent");
    expect(api.kind.value).toBe("expense");
    expect(api.type.value).toBe("rent");
  });

  it("keeps both params when two change in the same tick", async () => {
    const { router, api } = await setup();
    api.kind.value = "expense";
    api.type.value = "rent";
    await settle();
    expect(router.currentRoute.value.query).toEqual({ kind: "expense", type: "rent" });
  });

  it("drops a param that returns to its default and follows back/forward", async () => {
    const { router, api } = await setup("/list?kind=expense");
    api.kind.value = "all";
    await settle();
    expect(router.currentRoute.value.query.kind).toBeUndefined();
    await router.push("/list?kind=income");
    await settle();
    expect(api.kind.value).toBe("income");
  });
});

describe("navigation helpers", () => {
  it("only returns to in-app paths", () => {
    expect(backTarget({ back: "/payments?m=2026-07" }, "/x")).toBe("/payments?m=2026-07");
    expect(backTarget({ back: "https://evil.example" }, "/x")).toBe("/x");
    expect(backTarget({ back: "//evil.example" }, "/x")).toBe("/x");
  });
  it("adds a query param with or without an existing query", () => {
    expect(withQuery("/payments", "focus", "t1")).toBe("/payments?focus=t1");
    expect(withQuery("/payments?m=2026-09", "focus", "t1")).toBe("/payments?m=2026-09&focus=t1");
  });
});
