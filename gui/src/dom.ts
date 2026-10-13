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
  // The jingle mark, from gui/assets/jingle.svg.
  logo: () =>
    svg(
      "M12.8 19.2C12.8 15.6654 9.93462 12.8 6.4 12.8C2.86538 12.8 0 15.6654 0 19.2V25.6V38.4V44.8C0 48.3346 2.86538 51.2 6.4 51.2C9.93462 51.2 12.8 48.3346 12.8 44.8C12.8 41.2654 15.6654 38.4 19.2 38.4C22.7346 38.4 25.6 41.2654 25.6 44.8V51.2V57.6C25.6 61.1346 28.4654 64 32 64C35.5346 64 38.4 61.1346 38.4 57.6V51.2V44.8C38.4 41.2654 41.2654 38.4 44.8 38.4C48.3346 38.4 51.2 41.2654 51.2 44.8C51.2 48.3346 54.0654 51.2 57.6 51.2C61.1346 51.2 64 48.3346 64 44.8V38.4V25.6V19.2C64 15.6654 61.1346 12.8 57.6 12.8C54.0654 12.8 51.2 15.6654 51.2 19.2C51.2 22.7346 48.3346 25.6 44.8 25.6C41.2654 25.6 38.4 22.7346 38.4 19.2V12.8V6.4C38.4 2.86538 35.5346 0 32 0C28.4654 0 25.6 2.86538 25.6 6.4V12.8V19.2C25.6 22.7346 22.7346 25.6 19.2 25.6C15.6654 25.6 12.8 22.7346 12.8 19.2Z",
      false,
      "0 0 64 64",
    ),
};

function svg(d: string, stroke = false, viewBox = "0 0 16 16"): SVGElement {
  const ns = "http://www.w3.org/2000/svg";
  const s = document.createElementNS(ns, "svg");
  s.setAttribute("viewBox", viewBox);
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
