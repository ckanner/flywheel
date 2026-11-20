import { readFileSync } from 'node:fs'
import { runMachine } from '../../lib/machine.js'
const {request,runtime}=JSON.parse(readFileSync(process.argv[2],'utf8'))
await runMachine(request,runtime)
