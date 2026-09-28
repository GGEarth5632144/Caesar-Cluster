// คัดลอกข้อความ — navigator.clipboard มีเฉพาะ secure context (https / localhost)
// หน้าเว็บถูกเปิดผ่าน http://<ip> ของคลัสเตอร์ด้วย จึงต้องมีทางสำรองแบบเก่าไว้เสมอ
// แยกจาก error ของ API เพื่อให้ปุ่ม copy แสดงข้อความแนะนำของมันเองได้
export class ClipboardError extends Error {}

export async function copyText(text: string): Promise<void> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return
    }
  } catch {
    // secure context แต่ถูกปฏิเสธ (permission / แท็บไม่ได้โฟกัส) — ตกไปใช้ทางสำรอง
  }
  if (!copyWithExecCommand(text)) {
    throw new ClipboardError("คัดลอกไม่สำเร็จ — กรุณาลากเลือกข้อความแล้วกด Ctrl+C")
  }
}

// textarea ซ่อนนอกจอ + execCommand: ทำงานบน http ได้ แต่ต้องอยู่ใน event ของผู้ใช้ (คลิก/กดปุ่ม)
function copyWithExecCommand(text: string): boolean {
  const ta = document.createElement("textarea")
  ta.value = text
  ta.setAttribute("readonly", "")
  // ต้องอยู่ใน DOM และห้าม display:none ถึงจะ select ได้ · top:0 กันหน้าเลื่อนตอน focus
  ta.style.cssText = "position:fixed;top:0;left:-9999px;opacity:0;"
  document.body.appendChild(ta)

  const selection = document.getSelection()
  const previous = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null

  ta.select()
  ta.setSelectionRange(0, ta.value.length) // iOS ไม่สนใจ select() อย่างเดียว

  let ok = false
  try {
    ok = document.execCommand("copy")
  } catch {
    ok = false
  }

  document.body.removeChild(ta)
  if (previous && selection) {
    selection.removeAllRanges()
    selection.addRange(previous)
  }
  return ok
}
