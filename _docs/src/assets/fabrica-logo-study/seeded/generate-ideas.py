import secrets,json,sys
from pathlib import Path
metaphors=['a seed becoming a machine','a precision tooling jig','paper folded into a useful structure','a modular construction toy','an architectural arch','a lens focusing light','thread becoming fabric','a chisel carving material','a comet at ignition','a switch connecting energy','a makers stamp','a friendly abstract workshop creature','a tightly tied useful knot','magnetic attraction','a musical tuning fork','a structure growing from one pixel']
forms=['one bold compact silhouette','two unequal complementary pieces','three repeated solid elements','an expressive asymmetric shape','a block with one large negative-space cut','a continuous broad ribbon','a stepped pixel-grid silhouette','a soft monolithic form','an open geometric frame','a compact vertical totem']
characters=['radical retro-computing charm','Swiss precision and restraint','playful rebellious workshop energy','architectural weight and permanence','futuristic experimental typography without letters','organic growth and curiosity','brutalist industrial confidence','kinetic tension and release','quiet order and simplicity','unexpected friendly personality']
seed=sys.argv[1] if len(sys.argv)>1 else secrets.token_hex(32)
assert len(seed)==64 and all(c in '0123456789abcdef' for c in seed), 'Expected 64 lowercase hex characters'
remaining=[metaphors.copy(),forms.copy(),characters.copy()]
rows=[]
for i in range(10):
 chunk=seed[i*6:i*6+6]
 values=[int(chunk[j:j+2],16) for j in [0,2,4]]
 picks=[pool.pop(v%len(pool)) for pool,v in zip(remaining,values)]
 rows.append(dict(index=i+1,chunk=chunk,bytes=values,metaphor=picks[0],form=picks[1],character=picks[2]))
last=seed[60:]
finish=['crisp square-cut terminals','subtle rounded terminals','a deliberate single diagonal accent','one unexpected offset'][int(last,16)%4]
data=dict(seed=seed,algorithm='Read ten consecutive six-character chunks as three bytes. For each dimension select byte modulo remaining list length, then remove that choice, so each concept uses a different combination. The final four characters select a shared finishing constraint modulo four.',tables=dict(metaphors=metaphors,forms=forms,characters=characters),lastFour=last,finishingConstraint=finish,concepts=rows)
Path('/tmp/fabrica-seeded-ideas.json').write_text(json.dumps(data,indent=2))
print(json.dumps(data,indent=2))
