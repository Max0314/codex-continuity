import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';

test('account trigger has a mobile name and escape/outside dismissal; dialogs use native focus containment',async()=>{
 const source=await readFile(new URL('../src/App.tsx',import.meta.url),'utf8');
 assert.match(source,/aria-label=\{`账号：\$\{user.displayName\}`\}/);
 assert.match(source,/aria-expanded=\{accountOpen\}/);
 assert.match(source,/accountMenu\.current\?\.contains/);
 assert.match(source,/dialog\.showModal\(\)/);
 assert.match(source,/dialog\.close\(\)/);
 assert.match(source,/previous\?\.isConnected/);
 assert.match(source,/aria-label="关闭弹窗"/);
 assert.match(source,/onCancel=\{/);
});
