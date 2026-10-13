// A tiny element builder, enough for one settings window.

type Child = Node | string | null | undefined | false;
type Attrs = Record<string, string | number | boolean | EventListener | undefined | null>;

export function h(tag: string, attrs: Attrs = {}, ...children: (Child | Child[])[]): HTMLElement {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k.startsWith("on") && typeof v === "function") {
      el.addEventListener(k.slice(2), v);
    } else if (k === "value" && "value" in el) {
      (el as HTMLInputElement).value = String(v);
    } else if (k === "checked" && "checked" in el) {
      (el as HTMLInputElement).checked = Boolean(v);
    } else {
      el.setAttribute(k, v === true ? "" : String(v));
    }
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    el.append(c);
  }
  return el;
}

/** A labelled on/off switch. */
export function toggle(label: string, checked: boolean, onChange: (on: boolean) => void, disabled = false): HTMLElement {
  return h(
    "label",
    { class: "switch" + (disabled ? " disabled" : "") },
    h("input", {
      type: "checkbox",
      role: "switch",
      checked,
      disabled,
      "aria-label": label,
      onchange: (e: Event) => onChange((e.target as HTMLInputElement).checked),
    }),
    h("span", { class: "track", "aria-hidden": "true" }, h("span", { class: "thumb" })),
  );
}

export const icons = {
  play: () => svg("M5 3.5v9l8-4.5z"),
  remove: () => svg("M4.5 4.5l7 7m0-7l-7 7", true),
  plus: () => svg("M8 3v10M3 8h10", true),
};

function svg(d: string, stroke = false): SVGElement {
  const ns = "http://www.w3.org/2000/svg";
  const s = document.createElementNS(ns, "svg");
  s.setAttribute("viewBox", "0 0 16 16");
  s.setAttribute("aria-hidden", "true");
  const p = document.createElementNS(ns, "path");
  p.setAttribute("d", d);
  if (stroke) {
    p.setAttribute("fill", "none");
    p.setAttribute("stroke", "currentColor");
    p.setAttribute("stroke-width", "1.6");
    p.setAttribute("stroke-linecap", "round");
  } else {
    p.setAttribute("fill", "currentColor");
  }
  s.append(p);
  return s;
}
