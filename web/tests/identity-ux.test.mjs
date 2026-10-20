import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
test('member quick actions and unified accounts do not expose local user creation; device setup is graphical',async()=>{
 const source=await readFile(new URL('../src/App.tsx',import.meta.url),'utf8');
 assert.match(source,/user.role==='admin'&&!user.unified&&<QuickAction/);
 assert.match(source,/currentUser.role==='admin'&&!currentUser.unified\?<button/);
 assert.match(source,/user.avatarUrl/);assert.match(source,/Fribench 账号/);
 assert.doesNotMatch(source,/执行 init。/);assert.match(source,/在设置中选择‘连接 Fribench 账号’/);
});
