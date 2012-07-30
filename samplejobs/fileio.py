#!/usr/bin/python
#    Copyright (C) 2003-2010 Institute for Systems Biology
#                            Seattle, Washington, USA.
# 
#    This library is free software; you can redistribute it and/or
#    modify it under the terms of the GNU Lesser General Public
#    License as published by the Free Software Foundation; either
#    version 2.1 of the License, or (at your option) any later version.
# 
#    This library is distributed in the hope that it will be useful,
#    but WITHOUT ANY WARRANTY; without even the implied warranty of
#    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
#    Lesser General Public License for more details.
# 
#    You should have received a copy of the GNU Lesser General Public
#    License along with this library; if not, write to the Free Software
#    Foundation, Inc., 59 Temple Place, Suite 330, Boston, MA 02111-1307  USA
# 
#
""" This is a sample golem job that generates random numbers and prints them to files.
	
	From the comand line it can be ran  the
	sample, line and job ids:
	fileio.py 0 0 0
	
	To run it six times using a golem master listening at
	localhost:8083:
	
	golem.py localhost:8083 run 6 ~/Code/golem/src/samplejobs/fileio.py


"""
import random
import sys

#concactante the three args, the submission, line and job id's into one string for use 
#constructing filenames
id = ".".join(sys.argv[1:])

# this could be the absolute path, otherwise it ends up where ever you started the nodes
fo = open(id+".output.txt","w")
for i in range(1000):
	fo.write("%i\n"%(random.randint(0, 1000)))
fo.close()