/** Save the visible document as a multi-page A4 PDF. The rendered image keeps
 * member names and other Unicode text exactly as shown on screen. */
export async function downloadElementPdf(element: HTMLElement, filename: string, lightMode = false): Promise<void> {
  const [{ toCanvas }, { jsPDF }] = await Promise.all([import("html-to-image"), import("jspdf")]);
  await document.fonts.ready;
  if (lightMode) element.classList.add("pdf-light");
  let canvas: HTMLCanvasElement;
  try {
    canvas = await toCanvas(element, {
      backgroundColor: "#ffffff",
      pixelRatio: Math.min(2, 28000 / Math.max(element.scrollHeight, 1)),
      cacheBust: true,
      filter: (node) => !(node instanceof HTMLElement && node.hasAttribute("data-print-hide")),
    });
  } finally {
    if (lightMode) element.classList.remove("pdf-light");
  }
  const pdf = new jsPDF({ orientation: "portrait", unit: "mm", format: "a4", compress: true });
  const margin = 10;
  const pageW = 210 - margin * 2;
  const pageH = 297 - margin * 2;
  const slicePx = Math.max(1, Math.floor(canvas.width * pageH / pageW));
  const isBlank = (row: number) => {
    const data = canvas.getContext("2d")?.getImageData(0, row, canvas.width, 1).data;
    if (!data) return false;
    for (let x = 0; x < canvas.width; x += 3) {
      const i = x * 4;
      if (data[i] < 247 || data[i + 1] < 247 || data[i + 2] < 247) return false;
    }
    return true;
  };
  let contentEnd = canvas.height;
  while (contentEnd > 1 && isBlank(contentEnd - 1)) contentEnd -= 4;
  contentEnd = Math.min(canvas.height, contentEnd + 36);
  for (let y = 0, page = 0; y < contentEnd; page++) {
    let end = Math.min(y + slicePx, contentEnd);
    if (end < contentEnd) {
      for (let row = end - 12; row > y + slicePx * 0.65; row -= 4) {
        if (isBlank(row) && isBlank(Math.max(y, row - 12))) { end = row; break; }
      }
    }
    const h = end - y;
    const slice = document.createElement("canvas");
    slice.width = canvas.width;
    slice.height = h;
    const ctx = slice.getContext("2d");
    if (!ctx) throw new Error("PDF canvas is unavailable");
    ctx.drawImage(canvas, 0, y, canvas.width, h, 0, 0, canvas.width, h);
    if (page) pdf.addPage();
    pdf.addImage(slice.toDataURL("image/png"), "PNG", margin, margin, pageW, h * pageW / canvas.width);
    y = end;
  }
  pdf.save(filename);
}
