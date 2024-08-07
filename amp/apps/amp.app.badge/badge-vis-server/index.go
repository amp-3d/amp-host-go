package main

const (
	index = `<html>
	<head>
	<title>pic256</title>
	<style type="text/css">
	
		body {font-family:Helvetica,Arial,Calibri,sans-serif; margin-left: 5%;}
		img {padding-right: 1em;}
		h1 { font-size: 250%;}
		h2 { font-size: 150%; color:rgb(100,100,100);}
		h3 { font-size: 120%; color:rgb(120,120,120);}
		figure {
			font-family:Inconsolata,Courier,monospace; 
			margin-left:0em; display: 
			inline-block; 
			text-align: center; 
			font-size:120%; 
			color: rgb(120,120,120)
		}
	
	</style>
	</head>
	<body>
		<h1>Pic256</h1>
		<h2>Programmed pictures in a 256x256 square</h2>
		<h2>Defaults</h2>
	
		<figure><img width="256" height="256" src="/rotext/"/><figcaption>rotext</figcaption></figure>
		<figure><img width="256" height="256" src="/flower/"/><figcaption>flower</figcaption></figure>
		<figure><img width="256" height="256" src="/cube/"/><figcaption>cube</figcaption></figure>
		<figure><img width="256" height="256" src="/funnel/"/><figcaption>funnel</figcaption></figure>
		<figure><img width="256" height="256" src="/rshape/"/><figcaption>rshape</figcaption></figure>
		<figure><img width="256" height="256" src="/lewitt/"/><figcaption>lewitt</figcaption></figure>
		<figure><img width="256" height="256" src="/mondrian/"/><figcaption>mondrian</figcaption></figure>
		<figure><img width="256" height="256" src="/face/"/><figcaption>face</figcaption></figure>
		<figure><img width="256" height="256" src="/clock/"/><figcaption>clock</figcaption></figure>
		<figure><img width="256" height="256" src="/pacman/"/><figcaption>pacman</figcaption></figure>
		<figure><img width="256" height="256" src="/tux/"/><figcaption>tux</figcaption></figure>
		<figure><img width="256" height="256" src="/ubuntu/"/><figcaption>ubuntu</figcaption></figure>
		
		<h2>Variations</h2>
		
		<h3>tag.BADGE</h3>
		
		<p>
		<figure><img width="410" height="120" src="/hello/?petals=10&n=100"/><figcaption>petals=10&n=100</figcaption></figure>
		<figure><img width="410" height="120" src="/hello/?petals=15&n=50"/><figcaption>petals=15&n=50</figcaption></figure>
		<figure><img width="410" height="120" src="/hello/?petals=20&n=20"/><figcaption>petals=30&n=20</figcaption></figure>
		<figure><img width="410" height="120" src="/hello/?petals=30&n=10"/><figcaption>petals=30&n=10</figcaption></figure>
		</p>
		
		<h3>rotext</h3>
		<p>
		<figure><img width="256" height="256" src="/rotext/?char=a"/><figcaption>char=a</figcaption></figure>
		<figure><img width="256" height="256" src="/rotext/?char=b&ti=40"/><figcaption>char=b&ti=40</figcaption></figure>
		<figure><img width="256" height="256" src="/rotext/?char=c&ti=60"/><figcaption>char=c&ti=60</figcaption></figure>	
		<figure><img width="256" height="256" src="/rotext/?char=i&ti=90&font=Courier"/><figcaption>char=d&ti=90&font=Courier</figcaption></figure>	
		</p>
		
		
		<h3>flower</h3>
		<p>
		<figure><img width="256" height="256" src="/flower/?petals=10&n=100"/><figcaption>petals=10&n=100</figcaption></figure>
		<figure><img width="256" height="256" src="/flower/?petals=15&n=50"/><figcaption>petals=15&n=50</figcaption></figure>
		<figure><img width="256" height="256" src="/flower/?petals=20&n=20"/><figcaption>petals=30&n=20</figcaption></figure>
		<figure><img width="256" height="256" src="/flower/?petals=30&n=10"/><figcaption>petals=30&n=10</figcaption></figure>
		</p>

		<h3>cube</h3>
		<p>
		<figure><img width="256" height="256" src="/cube/?y=80&row=1"/><figcaption>y=80&row=1</figcaption></figure>
		<figure><img width="256" height="256" src="/cube/?y=50&row=2"/><figcaption>y=50&row=2</figcaption></figure>
		<figure><img width="256" height="256" src="/cube/?row=3"/><figcaption>&row=3</figcaption></figure>
		<figure><img width="256" height="256" src="/cube/?y=0&row=4"/><figcaption>y=0&row=4</figcaption></figure>
		</p>

		<h3>funnel</h3>
		<p>
		<figure><img width="256" height="256" src="/funnel/?step=10"/><figcaption>step=10</figcaption></figure>
		<figure><img width="256" height="256" src="/funnel/?step=15"/><figcaption>step=15</figcaption></figure>
		<figure><img width="256" height="256" src="/funnel/?step=25"/><figcaption>step=25</figcaption></figure>
		<figure><img width="256" height="256" src="/funnel/?step=25&bg=white&fg=black"/><figcaption>step=25&bg=white&fg=black</figcaption></figure>
		</p>
		
		<h3>rshape</h3>
		<p>
		<figure><img width="256" height="256" src="/rshape/?shape=c"/><figcaption>shape=c</figcaption></figure>
		<figure><img width="256" height="256" src="/rshape/?shape=r"/><figcaption>shape=r</figcaption></figure>
		<figure><img width="256" height="256" src="/rshape/?same=f"/><figcaption>same=f</figcaption></figure>
		<figure><img width="256" height="256" src="/rshape/?shape=r&same=t"/><figcaption>shape=r&same=t</figcaption></figure>
		</p>
		
		<h3>lewitt</h3>
		<p>
		<figure><img width="256" height="256" src="/lewitt/?pen=1&lines=20"/><figcaption>pen=1&lines=20</figcaption></figure>
		<figure><img width="256" height="256" src="/lewitt/?pen=2&lines=30"/><figcaption>pen=2&lines=30</figcaption></figure>
		<figure><img width="256" height="256" src="/lewitt/?pen=3&lines=40"/><figcaption>pen=3&lines=40</figcaption></figure>
		<figure><img width="256" height="256" src="/lewitt/?pen=5&?lines=100"/><figcaption>pen=5&?lines=100</figcaption></figure>
		</p>
		
	
		<h3>mondrian</h3>
		<p>
		<figure><img width="256" height="256" src="/mondrian/?random=true"/><figcaption>random=true</figcaption></figure>
		<figure><img width="256" height="256" src="/mondrian/?random=t"/><figcaption>random=t</figcaption></figure>
		<figure><img width="256" height="256" src="/mondrian/?random=1"/><figcaption>random=1</figcaption></figure>
		<figure><img width="256" height="256" src="/mondrian/?random=f"/><figcaption>random=f</figcaption></figure>
		</p>
		
		<h3>face</h3>
		<p>
		<figure><img width="256" height="256" src="/face/?mood=happy&glance=u"/><figcaption>mood=happy&glance=u</figcaption></figure>
		<figure><img width="256" height="256" src="/face/?mood=neutral&glance=d"/><figcaption>mood=neutral&glance=d</figcaption></figure>
		<figure><img width="256" height="256" src="/face/?mood=sad&glance=l"/><figcaption>mood=sad&glance=l</figcaption></figure>
		<figure><img width="256" height="256" src="/face/?mood=happy&glance=r"/><figcaption>mood=happy&glance=r</figcaption></figure>
		</p>

		<h3>clock</h3>
		<p>
                <figure><img width="256" height="256" src="/clock/"/><figcaption>clock</figcaption></figure>
                <figure><img width="256" height="256" src="/clock/?hour=23"/><figcaption>hour=23</figcaption></figure>
                <figure><img width="256" height="256" src="/clock/?hour=12&min=34"/><figcaption>hour=12&min=34</figcaption></figure>
                <figure><img width="256" height="256" src="/clock/?hour=6&min=30&sec=0"/><figcaption>hour=6&min=30&sec=0</figcaption></figure>
		</p>

		<h3>pacman</h3>
		<p>
		<figure><img width="256" height="256" src="/pacman/"/><figcaption>pacman</figcaption></figure>
		<figure><img width="256" height="256" src="/pacman/?angle=10"/><figcaption>angle=10</figcaption></figure>
		<figure><img width="256" height="256" src="/pacman/?angle=40"/><figcaption>angle=40</figcaption></figure>
		<figure><img width="256" height="256" src="/pacman/?angle=60"/><figcaption>angle=60</figcaption></figure>
		</p>

	</body>
</html>
`
)
