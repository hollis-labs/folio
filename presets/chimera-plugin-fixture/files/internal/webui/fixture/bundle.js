import {createElement,useState} from 'react';
import {Button} from '@recipe/ui';
export function Summary(){const [count,setCount]=useState(0);return createElement(Button,{className:'recipe-plugin p-7',onClick:()=>setCount(count+1)},'Reviewed widget local count '+count)}
