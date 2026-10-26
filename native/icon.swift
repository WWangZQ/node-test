import AppKit
let out=CommandLine.arguments[1]
let size=1024
let img=NSImage(size:NSSize(width:size,height:size))
img.lockFocus()
NSColor(calibratedRed:0.93,green:0.95,blue:0.89,alpha:1).setFill()
NSBezierPath(roundedRect:NSRect(x:35,y:35,width:954,height:954),xRadius:210,yRadius:210).fill()
NSColor(calibratedRed:0.19,green:0.34,blue:0.24,alpha:1).setFill()
NSBezierPath(roundedRect:NSRect(x:175,y:170,width:674,height:684),xRadius:55,yRadius:55).fill()
let colors:[NSColor]=[NSColor(calibratedRed:0.68,green:0.86,blue:0.53,alpha:1),NSColor(calibratedRed:0.95,green:0.82,blue:0.48,alpha:1),NSColor(calibratedRed:0.91,green:0.46,blue:0.51,alpha:1)]
for r in 0..<4{for c in 0..<3{colors[(r+c)%3].setFill();NSBezierPath(roundedRect:NSRect(x:220+c*200,y:228+r*140,width:180,height:110),xRadius:12,yRadius:12).fill()}}
img.unlockFocus()
let rep=NSBitmapImageRep(data:img.tiffRepresentation!)!
try rep.representation(using:.png,properties:[:])!.write(to:URL(fileURLWithPath:out))
